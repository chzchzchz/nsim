#!/usr/bin/env python3
"""Chart every numeric plant sensor after applying a command file.

Usage:
  chart_all.py --cmd <cmdfile> [--plant <plant.json>] [--scenario <id>]
               [--seconds <total>] [--step <seconds>] [--out <dir>]
               [--sensors '<pattern>[,<pattern>...]']

  When --seconds is omitted the run covers the last command's offset
  plus one step, so `--cmd <cmdfile> --plant <plant.json>` alone is
  enough to reproduce a recorded session end to end.

Design (why this shape):

- nsimrun emits every snapshot in one process via -snapshot-step,
  so Python just parses the JSONL stream and charts it. A single
  full-duration run applies each command at its absolute offset;
  the old per-chunk restart model (one nsimrun per snapshot chained
  through -plant files, with windowed command sub-files) is gone.
  Polling two-jsonles, re-parsing JSON, and fanning out over a
  thousand series remains Python-shaped work.
- Sensors are discovered by walking the first snapshot. Any scalar
  number or boolean is a plottable series; strings and arrays are
  skipped (mode labels, names). Booleans plot as 0/1 steps so switchover
  state (pumps running, valves open) stays visible.
- Rendering prefers matplotlib when it is installed and otherwise
  falls back to a hand-rolled SVG writer, so batch charting works on
  a machine with only the standard library. The script never installs
  anything itself; install matplotlib if you want PNG output with axes.
- When neither --seconds nor --cmd is given there is no trajectory
  to chart, which is an error. With --cmd alone the run covers the
  last command's offset plus one step.
"""
from __future__ import annotations

import argparse
import atexit
import contextlib
import html
import json
import math
import os
import shutil
import subprocess
import sys
import tempfile


def _walk(node, path=(), out=None):
    if out is None:
        out = {}
    if isinstance(node, dict):
        for k, v in node.items():
            _walk(v, path + (str(k),), out)
    else:
        out[".".join(path)] = node
    return out


def _parse_args(argv=None):
    p = argparse.ArgumentParser(description="Chart every sensor with a command file applied.")
    p.add_argument("--cmd", default="", help="command file (or web export *.json)")
    p.add_argument("--plant", default="", help="initial plant JSON (default: sim defaults)")
    p.add_argument("--scenario", default="", help="scenario id (mutually exclusive with --plant)")
    p.add_argument("--seconds", type=float, default=None, help="total seconds (default: last command + 1 step)")
    p.add_argument("--step", type=float, default=1.0, help="seconds between snapshots (>= 0.001)")
    p.add_argument("--out", default="chart_output", help="output directory")
    p.add_argument("--sensors", default="", help="comma-separated jq-style patterns; '*' matches any chars")
    p.add_argument("--bin", default="", help="path to prebuilt nsimrun binary (default: go build)")
    p.add_argument("--keep-json", action="store_true", help="keep per-step snapshots")
    p.add_argument("--no-index", action="store_true", help="skip writing index.html into the out dir")
    p.add_argument("--list", action="store_true", help="list discovered sensors and exit")
    return p.parse_args(argv)


def _parse_cmdfile(path):
    """Return (commands, scenario_id, max_offset_ms).

    Each command is a raw line that nsimrun understands; offsets are
    kept verbatim so reformatting cannot drift the schedule.
    """
    if path.lower().endswith(".json"):
        doc = json.load(open(path))
        cmds = []
        max_ms = 0
        for c in doc.get("commands", []):
            off = int(c.get("offsetMs", 0))
            max_ms = max(max_ms, off)
            cmds.append("%d %s %s" % (off, c.get("control", ""), c.get("value", "")))
        return cmds, str(doc.get("scenario", "")), max_ms
    cmds = []
    max_ms = 0
    for raw in open(path):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        if len(parts) < 3:
            continue
        try:
            off = int(float(parts[0]))
        except ValueError:
            continue
        max_ms = max(max_ms, off)
        cmds.append(line)
    return cmds, "", max_ms


def _match(patterns, name):
    if not patterns:
        return True
    for pat in patterns:
        pat = pat.lstrip(".")
        if pat.endswith("*"):
            if name.startswith(pat[:-1]):
                return True
        elif pat == name or name.startswith(pat + "."):
            return True
    return False


def _fmt_num(v):
    if math.isinf(v) or math.isnan(v):
        return str(v)
    if v == int(v) and abs(v) < 1e15:
        return str(int(v))
    return "%.6g" % v


def _draw_svg(name, times, series, y_title, path, is_bool=False, is_step=False):
    w, h = 1200, 600
    pad_l, pad_r, pad_t, pad_b = 90, 30, 60, 70
    pw, ph = w - pad_l - pad_r, h - pad_t - pad_b
    finite = [v for v in series if v is not None and math.isfinite(v)]
    if not finite:
        return
    t0, t1 = min(times), max(times)
    y0, y1 = min(finite), max(finite)
    if y0 == y1:
        y0 -= 1.0
        y1 += 1.0
    if not is_bool:
        span = y1 - y0
        y0 -= 0.05 * span
        y1 += 0.05 * span
    t_span = (t1 - t0) or 1.0

    def x(t):
        return pad_l + (t - t0) / t_span * pw

    def y(v):
        return pad_t + (y1 - v) / (y1 - y0) * ph

    grid = []
    for frac in (0.0, 0.25, 0.5, 0.75, 1.0):
        val = y0 + frac * (y1 - y0)
        yy = y(val)
        grid.append(
            '<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#ddd"/>' % (pad_l, yy, pad_l + pw, yy)
            + '<text x="%d" y="%.1f" text-anchor="end" font-size="11">%s</text>'
            % (pad_l - 6, yy + 4, _fmt_num(val))
        )
    for i in range(6):
        t = t0 + t_span * i / 5.0
        xx = x(t)
        grid.append(
            '<text x="%.1f" y="%d" text-anchor="middle" font-size="11">%s</text>'
            % (xx, h - pad_b + 20, _fmt_num(t))
        )

    if is_bool or is_step:
        pts = []
        prev = None
        for t, v in zip(times, series):
            if v is None or not math.isfinite(v):
                continue
            vv = 1.0 if v else 0.0
            if prev is not None and vv != prev:
                pts.append((x(t), y(prev)))
                pts.append((x(t), y(vv)))
            else:
                pts.append((x(t), y(vv)))
            prev = vv
        if pts:
            d = "M " + " L ".join("%.1f,%.1f" % p for p in pts)
            trace = '<path d="%s" fill="none" stroke="#1f77b4" stroke-width="1.5"/>' % d
        else:
            trace = ""
    else:
        pts = [(x(t), y(v)) for t, v in zip(times, series) if v is not None and math.isfinite(v)]
        if pts:
            d = "M " + " L ".join("%.1f,%.1f" % p for p in pts)
            trace = '<path d="%s" fill="none" stroke="#1f77b4" stroke-width="1.5"/>' % d
        else:
            trace = ""

    svg = (
        '<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">'
        '<rect width="100%%" height="100%%" fill="white"/>'
        '<text x="%d" y="30" font-size="16" font-weight="bold">%s</text>'
        '<text x="%d" y="%d" font-size="12">%s</text>'
        '<text x="%d" y="%d" font-size="12">Time (s)</text>'
        '<text x="20" y="%d" font-size="12" text-anchor="middle" transform="rotate(-90 20 %d)">%s</text>'
        "%s%s</svg>"
        % (
            w, h, pad_l, _escape(name), pad_l + pw, pad_t, y_title,
            pad_l + pw / 2, h - 12, pad_t + ph / 2, pad_t + ph / 2, _escape(y_title),
            "".join(grid), trace,
        )
    )
    with open(path, "w") as fh:
        fh.write(svg)


def _escape(s):
    return html.escape(s, quote=True)


def _write_index(path, items, meta, summaries=None):
    """Write index.html: one block per chart, each opening the image.

    The list keeps every chart visible even when matplotlib is absent
    (items then point at .svg files). Meta lines show the run so the
    page stays meaningful when moved with the charts. Charts are
    grouped by top-level building in collapsible sections, and a
    summary table (min/max/final) sits at the top so a reader can
    spot what actually moved without opening every chart.
    """
    by_building = {}
    for name, fname in items:
        building = name.split(".")[0]
        by_building.setdefault(building, []).append((name, fname))
    rows = []
    for building in sorted(by_building):
        figs = []
        for name, fname in sorted(by_building[building]):
            href = html.escape(fname, quote=True)
            title = html.escape(name, quote=True)
            figs.append(
                '<figure><figcaption>%s</figcaption>'
                '<a href="%s"><img loading="lazy" src="%s" '
                'style="max-width:100%%"></a></figure>' % (title, href, href)
            )
        rows.append(
            "<details open><summary>%s (%d)</summary>%s</details>"
            % (html.escape(building), len(figs), "".join(figs))
        )
    table_rows = ""
    if summaries:
        trs = []
        for s in summaries:
            trs.append(
                "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>"
                % (
                    html.escape(s["name"]),
                    html.escape(s["min"]),
                    html.escape(s["max"]),
                    html.escape(s["final"]),
                )
            )
        table_rows = (
            "<h2>Sensor summary (min / max / final)</h2>"
            "<table><thead><tr><th>sensor</th><th>min</th><th>max</th>"
            "<th>final</th></tr></thead><tbody>%s</tbody></table>"
            % "".join(trs)
        )
    meta_lines = "".join(
        "<li>%s: <code>%s</code></li>" % (html.escape(k), html.escape(str(v)))
        for k, v in meta if v != ""
    )
    page = """<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>nsim sensor charts</title>
<style>
body{font-family:system-ui,sans-serif;margin:2em;max-width:900px}
figure{margin:0 0 2em}
figcaption{font-family:monospace;margin-bottom:0.25em}
input,select{font:inherit;margin:0.25em 0.5em 0.25em 0}
table{border-collapse:collapse;font-family:monospace;font-size:13px}
th,td{border:1px solid #999;padding:2px 6px;text-align:right}
th:first-child,td:first-child{text-align:left}
details{margin-bottom:1em}
summary{cursor:pointer;font-weight:bold}
details details summary{font-weight:normal}
</style></head><body>
<h1>nsim sensor charts</h1>
<ul>%s</ul>
<p><input id=filter type=search placeholder="filter sensors (e.g. Pressurizer)" size="40">
<select id=sort><option value=name>sort: name</option><option value=range>sort: by range</option></select>
<label><input type=checkbox id=changed checked> only show changed</label></p>
%s
<div id=charts>%s</div>
<script>
var changedOnly=function(){
  var on=document.getElementById('changed').checked;
  document.querySelectorAll('#charts figure').forEach(function(f){
    var n=f.querySelector('figcaption').textContent;
    var changed=false;
    document.querySelectorAll('table tbody tr').forEach(function(tr){
      if(tr.children[0].textContent===n && tr.children[1].textContent!==tr.children[3].textContent) changed=true;
    });
    f.style.display=on && !changed ? 'none':'';
  });
};
function apply(){
  var q=document.getElementById('filter').value.toLowerCase();
  var figs=[...document.querySelectorAll('#charts figure')];
  figs.forEach(f=>f.style.display=f.textContent.toLowerCase().includes(q)?'':'none');
  changedOnly();
}
document.getElementById('filter').addEventListener('input',apply);
document.getElementById('changed').addEventListener('change',changedOnly);
document.getElementById('sort').addEventListener('change',e=>{
  var host=document.getElementById('charts');
  var figs=[...host.querySelectorAll('figure')];
  figs.sort((a,b)=>a.textContent.localeCompare(b.textContent));
  host.replaceChildren(...figs);
});
changedOnly();
</script>
</body></html>
""" % (meta_lines, table_rows, "".join(rows))
    with open(path, "w") as fh:
        fh.write(page)


def _render(name, times, series, y_title, out_path, is_bool, is_step, plotter=None):
    if plotter is not None:
        mode = plotter(name, times, series, y_title, out_path, is_bool, is_step)
        if mode is not None:
            return mode
    _draw_svg(name, times, series, y_title, out_path, is_bool, is_step)
    return "svg"


def main(argv=None):
    opts = _parse_args(argv)
    if opts.scenario and opts.plant:
        print("error: --scenario and --plant are mutually exclusive", file=sys.stderr)
        return 2
    if not opts.cmd and opts.seconds is None:
        print(
            "error: nothing to chart: pass --cmd <cmdfile> (and usually --plant <plant.json>), "
            "or --seconds <total> with --cmd",
            file=sys.stderr,
        )
        return 2
    if opts.cmd and not os.path.isfile(opts.cmd):
        print("error: command file %r not found" % opts.cmd, file=sys.stderr)
        return 2
    if opts.plant and not os.path.isfile(opts.plant):
        print("error: plant file %r not found" % opts.plant, file=sys.stderr)
        return 2
    if opts.step <= 0:
        print("error: --step must be positive", file=sys.stderr)
        return 2

    os.makedirs(opts.out, exist_ok=True)
    opts.out = os.path.abspath(opts.out)
    # The built nsimrun binary lives outside the out dir and is removed
    # on exit, so --keep-json only leaves the snapshots asked for.
    workdir = tempfile.mkdtemp(prefix="chart_all_")
    atexit.register(shutil.rmtree, workdir, ignore_errors=True)

    cmds, file_scenario, max_ms = ([], "", 0)
    if opts.cmd:
        cmds, file_scenario, max_ms = _parse_cmdfile(opts.cmd)
    if opts.scenario and file_scenario and file_scenario != opts.scenario:
        print(
            "error: --scenario %r conflicts with command file scenario %r"
            % (opts.scenario, file_scenario),
            file=sys.stderr,
        )
        return 2
    total = opts.seconds
    if total is None:
        if not cmds:
            print("error: no --seconds and empty --cmd: pass --seconds", file=sys.stderr)
            return 2
        total = max_ms / 1000.0 + opts.step
    if total <= 0:
        print("error: --seconds must be positive", file=sys.stderr)
        return 2

    step = opts.step
    n_steps = max(1, int(math.ceil(total / step)))
    step = total / n_steps

    repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    if opts.bin:
        run_cmd = [os.path.abspath(opts.bin)]
    else:
        built = os.path.join(workdir, "nsimrun")
        with contextlib.redirect_stderr(sys.stderr):
            proc = subprocess.run(
                ["go", "build", "-o", built, "./cmd/nsimrun/"], cwd=repo_root
            )
        if proc.returncode != 0:
            print("error: go build ./cmd/nsimrun/ failed", file=sys.stderr)
            return 1
        run_cmd = [built]

    scenario = opts.scenario or file_scenario
    times = []
    snapshots = []

    # One nsimrun process for the whole trace. Go emits snapshots at
    # t=0, step, 2*step, ..., total (same recompute as above) and
    # applies the full command file at absolute offsets; the
    # {"Alarms": ...} trailer line is not a plant snapshot.
    args = list(run_cmd) + ["-seconds", _fmt_num(total), "-snapshot-step", _fmt_num(step)]
    if opts.cmd:
        args += ["-cmd", os.path.abspath(opts.cmd)]
    if opts.plant:
        args += ["-plant", os.path.abspath(opts.plant)]
    elif scenario:
        args += ["-scenario", scenario]
    try:
        proc = subprocess.run(args, capture_output=True, text=True, cwd=repo_root)
    except FileNotFoundError:
        print("error: failed to run %r" % args[0], file=sys.stderr)
        return 1
    if proc.returncode != 0:
        print(
            "error: nsimrun failed:\n%s" % proc.stderr.strip()[-2000:],
            file=sys.stderr,
        )
        return 1
    snapshots = []
    for line in proc.stdout.splitlines():
        if not line.strip():
            continue
        try:
            doc = json.loads(line)
        except json.JSONDecodeError as exc:
            print("error: nsimrun emitted invalid JSON: %s" % exc, file=sys.stderr)
            return 1
        # The {"Alarms": ...} trailer line is the only non-plant doc.
        if set(doc.keys()) == {"Alarms"}:
            continue
        snapshots.append(doc)
    if not snapshots:
        print("error: nsimrun emitted no snapshots", file=sys.stderr)
        return 1
    # Go emits one snapshot per cadence point (t=0, step, ..., total);
    # map them in order, pinning the last point to total.
    times = [round(i * step, 9) for i in range(len(snapshots) - 1)] + [round(total, 9)]
    if opts.keep_json:
        for t, snap in zip(times, snapshots):
            with open(os.path.join(opts.out, "%06.3f.json" % t), "w") as fh:
                json.dump(snap, fh, indent=1)

    # Discover sensors from the first snapshot.
    flat0 = _walk(snapshots[0]) if snapshots else {}
    patterns = [p.strip() for p in opts.sensors.split(",") if p.strip()]
    numeric = {}
    booleans = {}
    for name, val in flat0.items():
        if name == "Alarms" or name.startswith("Alarms."):
            continue
        if isinstance(val, bool):
            booleans[name] = True
        elif isinstance(val, (int, float)):
            numeric[name] = True
    selected = [n for n in sorted(set(numeric) | set(booleans)) if _match(patterns, n)]
    if not selected:
        print("error: no numeric sensors discovered", file=sys.stderr)
        return 1
    if opts.list:
        for n in selected:
            print(n)
        print("%d sensors" % len(selected), file=sys.stderr)
        return 0

    # Flatten each snapshot once and pull the selected sensors out of
    # it. The old loop re-walked the whole plant tree once per sensor
    # (sensors x snapshots tree walks), which dominated run time on
    # long traces; walking once per snapshot is sensors x snapshots
    # dict lookups instead. Min/max per sensor is tracked here too so
    # the mirror check below needs no second pass over the series.
    # Sensor lookup goes through a pre-split path trie per snapshot so
    # skipped subtrees (Alarms, unselected buildings) are never walked
    # and only the selected leaves do dict work.
    n_snap = len(snapshots)
    series_by = {name: [None] * n_snap for name in selected}
    bounds = {}  # name -> (lo, hi) over finite values, absent if none
    # Split each sensor name once into a path tuple; the snapshot walk
    # then descends only along paths that lead to a selected sensor.
    path_trie = {}
    for name in selected:
        node = path_trie
        for part in name.split("."):
            node = node.setdefault(part, {})
        node[None] = name
    for ti, snap in enumerate(snapshots):
        stack = [(snap, path_trie)]
        while stack:
            node, trie = stack.pop()
            if not isinstance(node, dict):
                name = trie.get(None)
                if name is not None:
                    if isinstance(node, bool):
                        num = 1.0 if node else 0.0
                    elif isinstance(node, (int, float)) and math.isfinite(node):
                        num = float(node)
                    else:
                        continue
                    series_by[name][ti] = num
                    if name in bounds:
                        lo, hi = bounds[name]
                        if num < lo:
                            lo = num
                        if num > hi:
                            hi = num
                        bounds[name] = (lo, hi)
                    else:
                        bounds[name] = (num, num)
                continue
            for k, v in node.items():
                child = trie.get(k)
                if child is not None:
                    stack.append((v, child))

    # Collapse metadata noise: constant value, and pure mirror images
    # of an already kept series, neither add information. Mirror
    # detection uses relative tolerance so floating-point drift in
    # derived quantities (e.g. psi vs bar mirrors) still collapses.
    #
    # Two mirrors must have nearly equal (min, max) within the
    # tolerance, so candidates with disjoint ranges are skipped before
    # any per-point work. Min/max/range are precomputed once per kept
    # series (from the extraction pass above) instead of being rebuilt
    # inside the pairwise loop, which made the old check O(S^2 x T)
    # min/max rebuilding. Kept series with identical tuples are hashed
    # so the common exact-duplicate case is O(1) per candidate; only
    # near-miss pairs reach the full per-point tolerance check.
    MIRROR_TOL = 1e-6
    kept = []
    kept_series = []
    kept_bounds = []  # (lo, hi, rng) parallel to kept_series
    kept_by_tuple = {}
    for name in selected:
        series = series_by[name]
        b = bounds.get(name)
        if b is None:
            continue
        lo, hi = b
        if lo == hi:
            continue
        rng = hi - lo
        tup = tuple(series)
        if tup in kept_by_tuple:
            continue
        dup = False
        for k, other in enumerate(kept_series):
            olo, ohi, orng = kept_bounds[k]
            tol = MIRROR_TOL * max(1.0, orng, rng)
            if abs(lo - olo) > tol or abs(hi - ohi) > tol:
                continue
            if all(
                a is not None and c is not None and abs(a - c) <= tol
                for a, c in zip(series, other)
            ):
                dup = True
                break
        if dup:
            continue
        kept.append(name)
        kept_series.append(series)
        kept_bounds.append((lo, hi, rng))
        kept_by_tuple[tup] = name
    if not kept:
        # Everything was constant; chart the first sensor anyway so
        # the user sees a flat line instead of an empty directory.
        kept = selected[:1]
        kept_series = [series_by[kept[0]]]

    charted = []
    n_png = n_svg = 0
    plotter = _plotter()
    ext = "png" if plotter is not None else "svg"
    summaries = []
    for name, series in zip(kept, kept_series):
        finite = [v for v in series if v is not None and math.isfinite(v)]
        summaries.append({
            "name": name,
            "min": _fmt_num(min(finite)) if finite else "",
            "max": _fmt_num(max(finite)) if finite else "",
            "final": _fmt_num(finite[-1]) if finite else "",
            "bool": name in booleans,
        })
        safe = name.replace("/", "_")
        is_bool = name in booleans
        is_step = is_bool
        out_png = os.path.join(opts.out, safe + "." + ext)
        mode = _render(
            name, times, series, _y_title(name),
            out_png, is_bool, is_step, plotter,
        )
        if mode == "png":
            n_png += 1
        else:
            n_svg += 1
        charted.append((name, os.path.basename(out_png)))

    if not opts.no_index:
        meta = [
            ("cmd", opts.cmd or "(none)"),
            ("plant", opts.plant or "(defaults)"),
            ("scenario", scenario or "(none)"),
            ("total", "%gs" % total),
            ("step", "%gs" % step),
            ("snapshots", str(len(times))),
        ]
        _write_index(os.path.join(opts.out, "index.html"), charted, meta, summaries)

    print(
        "charted %d sensors from %d snapshots in %s (%d png, %d svg)"
        % (len(kept), len(times), opts.out, n_png, n_svg),
        file=sys.stderr,
    )
    return 0


def _y_title(name):
    return name.rsplit(".", 1)[-1]


def _has_no_matplotlib():
    try:
        import matplotlib  # noqa: F401
        return False
    except Exception:
        return True


def _plotter():
    """Return a render function, importing matplotlib once per process."""
    try:
        import matplotlib

        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
    except Exception:
        return None

    def render(name, times, series, y_title, out_path, is_bool, is_step):
        xs = [t for t, v in zip(times, series) if v is not None and math.isfinite(v)]
        ys = [v for v in series if v is not None and math.isfinite(v)]
        if not xs:
            return None
        plt.figure(figsize=(12, 6))
        if is_bool or is_step:
            plt.step(xs, ys, where="post")
        else:
            plt.plot(xs, ys)
        plt.title(name)
        plt.xlabel("Time (s)")
        plt.ylabel(y_title)
        plt.grid(True, alpha=0.3)
        plt.tight_layout()
        plt.savefig(out_path, dpi=100)
        plt.close()
        return "png"

    return render


if __name__ == "__main__":
    sys.exit(main())
