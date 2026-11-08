package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/chzchzchz/nsim/bridge"
	"github.com/chzchzchz/nsim/cmdfile"
	"github.com/chzchzchz/nsim/core"
	"github.com/chzchzchz/nsim/internal/plantinit"
)

// methodAllowed reports whether method is valid for an exact path.
// It is consulted before the mux so wrong-method requests get the
// uniform 405 envelope instead of Go's plain-text default.
func methodAllowed(r *http.Request) bool {
	exact := map[string][]string{
		"/livez":           {http.MethodGet},
		"/readyz":          {http.MethodGet},
		"/status":          {http.MethodGet},
		"/scenarios":       {http.MethodGet},
		"/sensors":         {http.MethodGet},
		"/commands/export": {http.MethodGet},
		"/logs":            {http.MethodGet, http.MethodPost},
		"/alarms":          {http.MethodGet},
		"/state":           {http.MethodGet},
		"/metrics":         {http.MethodGet},
		"/controls":        {http.MethodGet},
		"/commands":        {http.MethodGet, http.MethodPost},
		"/pause":           {http.MethodPost},
		"/resume":          {http.MethodPost},
		"/step":            {http.MethodPost},
		"/reset":           {http.MethodPost},
		"/timestep":        {http.MethodPost},
		"/speed":           {http.MethodPost},
	}
	if methods, ok := exact[r.URL.Path]; ok {
		return slices.Contains(methods, r.Method)
	}
	// Wildcard routes: /sensors/{key} (GET) and /controls/{key}
	// (GET, PUT). The key never appears without its prefix slash,
	// so a simple HasPrefix plus length check is exact.
	switch {
	case strings.HasPrefix(r.URL.Path, "/sensors/") && len(r.URL.Path) > len("/sensors/"):
		return r.Method == http.MethodGet
	case strings.HasPrefix(r.URL.Path, "/controls/") && len(r.URL.Path) > len("/controls/"):
		return r.Method == http.MethodGet || r.Method == http.MethodPut
	}
	// Unknown paths keep the fallthrough 404 handling.
	return true
}

// errorResponse is the uniform envelope for every non-2xx response.
type errorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: msg, Code: code})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

func writeSimpleBool(w http.ResponseWriter, key string, value bool) {
	writeJSON(w, http.StatusOK, map[string]bool{key: value})
}

// decodeJSONBody reads and JSON-decodes a request body with the
// shared size cap. Empty and whitespace-only bodies decode to the
// zero value of v, so callers that accept an empty default (POST
// /step, POST /reset) do not need a ContentLength check that
// misbehaves on chunked requests (ContentLength == -1).
func decodeJSONBody(r *http.Request, v any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if int64(len(raw)) > maxRequestBody {
		return fmt.Errorf("body too large")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("parse JSON body: %w", err)
	}
	return nil
}

// cors wraps the mux with the permissive CORS headers the future
// remote web UI needs, and answers preflight without touching the
// simulation.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler returns the API mux. Routes use Go 1.27 ServeMux patterns;
// control and sensor keys are wildcard-captured as {key...} so
// percent-encoded %2F slashes stay intact in the path.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", s.handleLivez)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /scenarios", s.handleScenarios)
	mux.HandleFunc("GET /sensors", s.handleSensors)
	mux.HandleFunc("GET /sensors/{key...}", s.handleSensor)
	mux.HandleFunc("GET /controls", s.handleControls)
	mux.HandleFunc("GET /controls/{key...}", s.handleControl)
	mux.HandleFunc("PUT /controls/{key...}", s.handlePutControl)
	mux.HandleFunc("GET /commands", s.handleCommands)
	mux.HandleFunc("POST /commands", s.handlePostCommands)
	mux.HandleFunc("GET /commands/export", s.handleCommandsExport)
	mux.HandleFunc("GET /logs", s.handleLogs)
	mux.HandleFunc("POST /logs", s.handlePostLog)
	mux.HandleFunc("POST /pause", s.handlePause)
	mux.HandleFunc("POST /resume", s.handleResume)
	mux.HandleFunc("POST /step", s.handleStep)
	mux.HandleFunc("POST /reset", s.handleReset)
	mux.HandleFunc("POST /timestep", s.handleTimestep)
	mux.HandleFunc("POST /speed", s.handleSpeed)
	mux.HandleFunc("GET /alarms", s.handleAlarms)
	mux.HandleFunc("GET /state", s.handleState)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	// Unknown paths share the uniform envelope.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown path %q", r.URL.Path))
	})
	return cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !methodAllowed(r) {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

func (s *Server) handleLivez(w http.ResponseWriter, _ *http.Request) {
	writeSimpleBool(w, "ok", true)
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("{}\n"))
		return
	}
	writeSimpleBool(w, "ready", true)
}

type objectiveView struct {
	Active bool   `json:"active"`
	ID     string `json:"id,omitempty"`
	Title  string `json:"title,omitempty"`
	Done   bool   `json:"done"`
	MetMs  int64  `json:"metMs"`
	Detail string `json:"detail,omitempty"`
}

type statusView struct {
	SiteName       string        `json:"siteName"`
	RunID          int64         `json:"runId"`
	SimTimeMs      int64         `json:"simTimeMs"`
	TimestepMs     float64       `json:"timestepMs"`
	StepsPerSecond int           `json:"stepsPerSecond"`
	Running        bool          `json:"running"`
	Paused         bool          `json:"paused"`
	Rev            int64         `json:"rev"`
	Objective      objectiveView `json:"objective"`
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Milestone latching happens in stepOnce under the write lock;
	// this handler only reads the progress view. Calling Observe here
	// would mutate shared tracker state under a read lock.
	obj := objectiveView{Active: false, MetMs: -1}
	if s.scenario != nil {
		res := s.progress.Result(s.plant)
		obj = objectiveView{
			Active: true,
			ID:     s.scenario.ID,
			Title:  s.scenario.Title,
			Done:   res.Done,
			MetMs:  s.progress.MetMs(),
			Detail: res.Detail,
		}
	}
	writeJSON(w, http.StatusOK, statusView{
		SiteName:       s.siteName,
		RunID:          s.runId,
		SimTimeMs:      s.simTimeNs / int64(time.Millisecond),
		TimestepMs:     float64(s.dt) / float64(time.Millisecond),
		StepsPerSecond: s.stepsPerSecond,
		Running:        !s.paused,
		Paused:         s.paused,
		Rev:            s.rev,
		Objective:      obj,
	})
}

func (s *Server) handleScenarios(w http.ResponseWriter, _ *http.Request) {
	type scenarioInfo struct {
		ID           string `json:"id"`
		Number       int    `json:"number"`
		Category     string `json:"category"`
		Title        string `json:"title"`
		StartState   string `json:"startState"`
		Objective    string `json:"objective"`
		DurationHint string `json:"durationHint,omitempty"`
	}
	out := make([]scenarioInfo, 0, len(core.Scenarios()))
	for _, sc := range core.Scenarios() {
		out = append(out, scenarioInfo{
			ID: sc.ID, Number: sc.Number, Category: sc.Category, Title: sc.Title,
			StartState: sc.StartState, Objective: sc.Objective, DurationHint: sc.DurationHint,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSensors(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	simTimeMs := s.simTimeNs / int64(time.Millisecond)
	out := make([]sensorEntry, 0, len(s.sensors))
	for _, key := range sortedKeys(s.sensors) {
		sensor := s.sensors[key]
		val, err := sensorValue(sensor)
		if err != nil {
			val = ""
		}
		out = append(out, sensorEntry{
			Key: key, Part: sensor.Part, Label: sensor.Label, Kind: sensor.Kind,
			Value: val, SimTimeMs: simTimeMs,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSensor(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeError(w, http.StatusNotFound, "missing sensor key")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sensor, ok := s.sensors[key]
	if !ok {
		// Resolve dot forms too so clients can use either separator.
		dotKey := strings.ReplaceAll(key, ".", "/")
		sensor, ok = s.sensors[dotKey]
	}
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown sensor %q", key))
		return
	}
	val, err := sensorValue(sensor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sensorEntry{
		Key: key, Part: sensor.Part, Label: sensor.Label, Kind: sensor.Kind,
		Value: val, SimTimeMs: s.simTimeNs / int64(time.Millisecond),
	})
}

func (s *Server) handleControls(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]controlEntry, 0, len(s.controls))
	for _, key := range sortedKeys(s.controls) {
		ctrl := s.controls[key]
		out = append(out, controlEntry{
			Key: key, Part: ctrl.Part, Label: ctrl.Label, Kind: ctrl.Kind,
			Enum: ctrl.Enum, Value: controlValue(ctrl),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeError(w, http.StatusNotFound, "missing control key")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	resolved := cmdfile.ResolveKey(s.controls, key)
	ctrl, ok := s.controls[resolved]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown control %q", key))
		return
	}
	writeJSON(w, http.StatusOK, controlEntry{
		Key: resolved, Part: ctrl.Part, Label: ctrl.Label, Kind: ctrl.Kind,
		Enum: ctrl.Enum, Value: controlValue(ctrl),
	})
}

func (s *Server) handlePutControl(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeError(w, http.StatusNotFound, "missing control key")
		return
	}
	var body struct {
		Value   string `json:"value"`
		Comment string `json:"comment,omitempty"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid control value: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	cmd := cmdfile.Command{ControlKey: key, Value: body.Value}
	if err := cmdfile.Apply(s.controls, cmd); err != nil {
		if errors.Is(err, cmdfile.ErrUnknownControl) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	resolved := cmdfile.ResolveKey(s.controls, key)
	// Journal unconditionally: momentary enum controls must fire on
	// every PUT. Dedup only governs batch re-submission.
	logged := cmdfile.Command{ControlKey: resolved, Value: body.Value, Comment: body.Comment}
	s.journal.Add(logged, s.simTimeNs)
	s.log.AddOperation(logged, s.simTimeNs, "")
	s.rev++
	writeJSON(w, http.StatusOK, controlEntry{
		Key: resolved, Part: s.controls[resolved].Part, Label: s.controls[resolved].Label,
		Kind: s.controls[resolved].Kind, Enum: s.controls[resolved].Enum,
		Value: controlValue(s.controls[resolved]),
	})
}

type scheduledEntry struct {
	Seq   int64 `json:"seq"`
	DueMs int64 `json:"dueMs"`
}

type commandsResponse struct {
	Scheduled []scheduledEntry `json:"scheduled"`
}

type commandsExport struct {
	Scenario string            `json:"scenario"`
	Commands []cmdfile.Command `json:"commands"`
}

type queueEntryView struct {
	Seq      int64  `json:"seq"`
	Text     string `json:"text,omitempty"`
	Control  string `json:"control"`
	Value    string `json:"value"`
	OffsetMs int64  `json:"offsetMs"`
	DueMs    int64  `json:"dueMs"`
	Status   string `json:"status"`
	Err      string `json:"err,omitempty"`
}

type commandsView struct {
	Rev     int64            `json:"rev"`
	Entries []queueEntryView `json:"entries"`
}

func (s *Server) handleCommands(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	entries := s.queue.Entries()
	rev := s.queue.Rev()
	s.mu.RUnlock()
	out := commandsView{Rev: rev, Entries: make([]queueEntryView, 0, len(entries))}
	for _, e := range entries {
		out.Entries = append(out.Entries, queueEntryView{
			Seq: e.Seq, Text: e.Text, Control: e.Cmd.ControlKey, Value: e.Cmd.Value,
			OffsetMs: e.Cmd.TimeOffsetMs, DueMs: e.DueNs / int64(time.Millisecond),
			Status: e.Status.String(), Err: e.Err,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCommandsExport(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cmds := s.journal.CommandsFrom(0)
	if cmds == nil {
		cmds = []cmdfile.Command{}
	}
	export := commandsExport{Commands: cmds}
	if s.scenario != nil {
		export.Scenario = s.scenario.ID
	}
	writeJSON(w, http.StatusOK, export)
}

// parseCommandBody decodes a POST /commands body into the shared
// envelope (JSON {commands:[...]}) or the line grammar, dispatching
// on Content-Type and sniffing a leading brace when the header is
// absent so curl -d @file.json works without an explicit content
// type.
func parseCommandBody(r *http.Request) ([]cmdfile.Command, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > maxRequestBody {
		return nil, fmt.Errorf("command body too large")
	}
	trimmed := bytes.TrimSpace(body)
	isJSON := bytes.HasPrefix(trimmed, []byte("{"))
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	switch {
	case strings.Contains(contentType, "application/json"), isJSON && !strings.Contains(contentType, "text/plain"):
		var env commandsExport
		if err := json.NewDecoder(bytes.NewReader(body)).Decode(&env); err != nil {
			return nil, fmt.Errorf("parse command JSON: %w", err)
		}
		if env.Commands == nil {
			return []cmdfile.Command{}, nil
		}
		return env.Commands, nil
	default:
		cmds, err := cmdfile.Parse(bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if cmds == nil {
			return []cmdfile.Command{}, nil
		}
		return cmds, nil
	}
}

func (s *Server) handlePostCommands(w http.ResponseWriter, r *http.Request) {
	cmds, err := parseCommandBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Validate every command before scheduling (all-or-nothing), and
	// resolve each command exactly once into the parallel resolved
	// slice so validation, dedup, and journaling never re-resolve
	// the same key through different paths.
	s.mu.Lock()
	resolved := make([]string, len(cmds))
	for i, c := range cmds {
		key := cmdfile.ResolveKey(s.controls, c.ControlKey)
		resolved[i] = key
		ctrl, ok := s.controls[key]
		if !ok {
			s.mu.Unlock()
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown control %q", c.ControlKey))
			return
		}
		if _, err := bridge.ConvertControlValue(ctrl.Kind, c.Value); err != nil {
			s.mu.Unlock()
			writeError(w, http.StatusBadRequest, fmt.Sprintf("set control %q to %q: %v", c.ControlKey, c.Value, err))
			return
		}
	}
	// Dedup is judged against the journal as it stood before this
	// batch; a repeat POST of an identical identity is a no-op for
	// the journal (the queue entry is still recorded for UI polling).
	deduped := make([]bool, len(cmds))
	for i, c := range cmds {
		dueNs := c.DueNs(s.simTimeNs)
		deduped[i] = s.journal.Has(resolved[i], c.Value, dueNs)
	}
	texts := make([]string, 0, len(cmds))
	for _, c := range cmds {
		if c.AtMs != nil {
			texts = append(texts, fmt.Sprintf("@%d %s %s", *c.AtMs, c.ControlKey, c.Value))
		} else {
			texts = append(texts, fmt.Sprintf("%d %s %s", c.TimeOffsetMs, c.ControlKey, c.Value))
		}
	}
	entries := s.queue.SubmitAll(cmds, texts, s.simTimeNs)
	out := make([]scheduledEntry, 0, len(entries))
	for i, e := range entries {
		dueMs := e.DueNs / int64(time.Millisecond)
		// Journal the resolved key, not the raw client spelling, so
		// Has/Add stay comparable across dot- and slash-form posts.
		if !deduped[i] {
			journalCmd := e.Cmd
			journalCmd.ControlKey = resolved[i]
			s.journal.Add(journalCmd, e.DueNs)
		}
		// The log records every submission, including ones the
		// journal dedupes: it is the audit trail, not the
		// replay list.
		logCmd := e.Cmd
		logCmd.ControlKey = resolved[i]
		s.log.AddOperation(logCmd, e.DueNs, "")
		out = append(out, scheduledEntry{Seq: e.Seq, DueMs: dueMs})
	}
	if len(entries) > 0 && allTrue(deduped) {
		dueNs := cmds[0].DueNs(s.simTimeNs)
		origSeq := s.journal.SeqOf(resolved[0], cmds[0].Value, dueNs)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"deduped": true, "seq": origSeq})
		return
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, commandsResponse{Scheduled: out})
}

// logView is the wire shape of one log entry.
type logView struct {
	Seq     int64         `json:"seq"`
	Kind    cmdfile.LogKind `json:"kind"`
	AtMs    int64         `json:"atMs"`
	Control string        `json:"control,omitempty"`
	Value   string        `json:"value,omitempty"`
	Comment string        `json:"comment,omitempty"`
	Source  string        `json:"source,omitempty"`
}

// handleLogs returns the append-only operation log: every
// command-file operation (with its attached comment) and every
// bare comment, ordered by simulated timestamp. Unlike the
// journal, the log never deduplicates, so re-submitting the
// same command appends a second entry.
func (s *Server) handleLogs(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := s.log.Entries()
	out := make([]logView, 0, len(entries))
	for _, e := range entries {
		out = append(out, logView{
			Seq: e.Seq, Kind: e.Kind, AtMs: e.AtMs,
			Control: e.Control, Value: e.Value,
			Comment: e.Comment, Source: e.Source,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePostLog appends a bare operator comment at the current
// simulated time. Comments are encouraged alongside operations
// so the log reads as an annotated record of the run.
func (s *Server) handlePostLog(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Comment string `json:"comment"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid log comment: %v", err))
		return
	}
	if strings.TrimSpace(body.Comment) == "" {
		writeError(w, http.StatusBadRequest, "comment must not be empty")
		return
	}
	s.mu.Lock()
	s.log.AddComment(body.Comment, s.simTimeNs)
	s.rev++
	atMs := s.simTimeNs / int64(time.Millisecond)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"comment": body.Comment, "atMs": atMs})
}

func allTrue(flags []bool) bool {
	for _, f := range flags {
		if !f {
			return false
		}
	}
	return len(flags) > 0
}

func (s *Server) handlePause(w http.ResponseWriter, _ *http.Request) {
	s.setPaused(w, true)
}

func (s *Server) handleResume(w http.ResponseWriter, _ *http.Request) {
	s.setPaused(w, false)
}

func (s *Server) setPaused(w http.ResponseWriter, paused bool) {
	s.mu.Lock()
	s.paused = paused
	s.rev++
	s.mu.Unlock()
	writeSimpleBool(w, "paused", paused)
}

func (s *Server) handleStep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Ms    int64 `json:"ms"`
		Steps int64 `json:"steps"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid step request: %v", err))
		return
	}
	// The paused check and the advance share one lock acquisition so
	// a resume landing between check and step cannot tear them.
	s.mu.Lock()
	if !s.paused {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "POST /step is only available while paused")
		return
	}
	var steps int
	switch {
	case body.Steps > 0:
		steps = int(min(body.Steps, maxStepRequest))
		for range steps {
			s.stepOnce()
		}
	case body.Ms > 0:
		steps = s.stepMsLocked(body.Ms)
	default:
		// Empty body defaults to a single step.
		s.stepOnce()
		steps = 1
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]int{"steps": steps})
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scenario string `json:"scenario"`
		Plant    string `json:"plant"`
	}
	// An empty body resets to the current startup lineup.
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid reset request: %v", err))
		return
	}
	s.ready.Store(false)
	defer s.ready.Store(true)
	// The plant rebuild happens under the write lock so /readyz
	// sees the server as unready for the whole transition.
	s.mu.Lock()
	defer s.mu.Unlock()
	scenarioID := body.Scenario
	if scenarioID == "" {
		scenarioID = s.scenarioID
	}
	plantPath := body.Plant
	if plantPath == "" {
		plantPath = s.plantPath
	}
	if err := s.resetLocked(scenarioID, plantPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Reload the startup command file so the reset run starts from
	// the same scheduling point as a fresh process launch.
	reloadCmds := []cmdfile.Command{}
	if s.cmdPath != "" {
		cmds, _, err := plantinit.LoadCommands(s.cmdPath)
		if err == nil {
			reloadCmds = cmds
		}
	}
	if len(reloadCmds) > 0 {
		texts := make([]string, len(reloadCmds))
		for i, c := range reloadCmds {
			texts[i] = fmt.Sprintf("%d %s %s", c.TimeOffsetMs, c.ControlKey, c.Value)
		}
		s.queue.SubmitAll(reloadCmds, texts, 0)
		for _, e := range s.queue.Entries() {
			s.journal.Add(e.Cmd, e.DueNs)
			s.log.AddOperation(e.Cmd, e.DueNs, "")
		}
	}
	writeJSON(w, http.StatusOK, map[string]int64{"runId": s.runId})
}

func (s *Server) handleTimestep(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DtMs float64 `json:"dtMs"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid timestep: %v", err))
		return
	}
	d, err := durationFromMs(body.DtMs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	s.dt = d
	s.rev++
	timestepMs := float64(s.dt) / float64(time.Millisecond)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]float64{"timestepMs": timestepMs})
}

func (s *Server) handleSpeed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		StepsPerSecond int `json:"stepsPerSecond"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid speed: %v", err))
		return
	}
	if body.StepsPerSecond <= 0 {
		writeError(w, http.StatusBadRequest, "stepsPerSecond must be positive")
		return
	}
	if body.StepsPerSecond > maxStepsPerSecond {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("stepsPerSecond exceeds maximum %d", maxStepsPerSecond))
		return
	}
	s.mu.Lock()
	s.stepsPerSecond = body.StepsPerSecond
	s.rev++
	sps := s.stepsPerSecond
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]int{"stepsPerSecond": sps})
}

func (s *Server) handleAlarms(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	alarms := s.plant.Alarms()
	if alarms == nil {
		alarms = []core.Alarm{}
	}
	writeJSON(w, http.StatusOK, alarms)
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := json.Marshal(s.plant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP nsim_server_info Server identity.\n")
	fmt.Fprintf(&b, "# TYPE nsim_server_info gauge\n")
	fmt.Fprintf(&b, "nsim_server_info{site=%q} 1\n", s.siteName)
	fmt.Fprintf(&b, "# HELP nsim_sim_time_seconds Simulated time in seconds.\n")
	fmt.Fprintf(&b, "# TYPE nsim_sim_time_seconds gauge\n")
	fmt.Fprintf(&b, "nsim_sim_time_seconds %g\n", float64(s.simTimeNs)/float64(time.Second))
	fmt.Fprintf(&b, "# HELP nsim_alarms_active Active alarm count.\n")
	fmt.Fprintf(&b, "# TYPE nsim_alarms_active gauge\n")
	fmt.Fprintf(&b, "nsim_alarms_active %d\n", len(s.plant.Alarms()))
	fmt.Fprintf(&b, "# HELP nsim_steps_total Simulation steps executed.\n")
	fmt.Fprintf(&b, "# TYPE nsim_steps_total counter\n")
	fmt.Fprintf(&b, "nsim_steps_total %d\n", s.stepsTotal)
	emitted := make(map[string]string, len(s.sensors))
	for _, key := range sortedKeys(s.sensors) {
		sensor := s.sensors[key]
		if sensor.Kind == "enum" {
			// Prometheus has no string value type; enum sensors are
			// omitted and remain readable via /sensors/{key}.
			continue
		}
		val, err := sensorValue(sensor)
		if err != nil {
			continue
		}
		num, err := parseFloat(val)
		if err != nil {
			continue
		}
		// Sanitization can collide two keys into one metric name;
		// later duplicates are dropped so the exposition stays
		// valid Prometheus (no repeated HELP/TYPE for one series).
		name := metricName(key)
		if firstKey, ok := emitted[name]; ok && firstKey != key {
			continue
		}
		emitted[name] = key
		labels := fmt.Sprintf("key=%q,part=%q,label=%q,unit=%q",
			key, sensor.Part, sensor.Label, metricUnit(sensor.Kind))
		fmt.Fprintf(&b, "# HELP %s %s %s sensor gauge.\n", name, sensor.Part, sensor.Label)
		fmt.Fprintf(&b, "# TYPE %s gauge\n", name)
		fmt.Fprintf(&b, "%s{%s} %g\n", name, labels, num)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
