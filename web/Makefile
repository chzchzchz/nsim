# nsim-web — Makefile
#
# Targets:
#   make build   — compile wasm, copy polyfill, run tsc, emit dist/
#   make wasm    — compile wasm + copy wasm_exec.js into public/
#   make wasm-exec — copy $(go env GOROOT)/lib/wasm/wasm_exec.js into public/ and src/
#   make typecheck — tsc --noEmit
#   make dev     — vite dev server on :5173
#   make clean   — remove public/nsim.wasm, dist/, src/wasm_exec.js

GO ?= go
GOROOT := $(shell $(GO) env GOROOT)
WASM_SRC := wasm
WASM_OUT := public/nsim.wasm
WASM_EXEC_DST_PUBLIC := public/wasm_exec.js
WASM_EXEC_DST_SRC := src/wasm_exec.js

.PHONY: all build wasm wasm-exec typecheck dev clean

all: build wasm-exec

build: wasm typecheck
	@echo "build complete (run \`make dev\` to serve, or use public/ + dist/ for static hosting)"

wasm: $(WASM_OUT)

$(WASM_OUT): $(WASM_SRC)/main.go ../core ../bridge ../core/svg
	GOOS=js GOARCH=wasm $(GO) build -o $(WASM_OUT) ./$(WASM_SRC)
	@echo "wrote $(WASM_OUT)"

wasm-exec: $(WASM_EXEC_DST_PUBLIC) $(WASM_EXEC_DST_SRC)

$(WASM_EXEC_DST_PUBLIC): $(WASM_EXEC_DST_SRC)
	cp $(WASM_EXEC_DST_SRC) $(WASM_EXEC_DST_PUBLIC)

$(WASM_EXEC_DST_SRC):
	cp $(GOROOT)/lib/wasm/wasm_exec.js $(WASM_EXEC_DST_SRC)

typecheck:
	@echo "running tsc --noEmit"
	@if command -v npx >/dev/null 2>&1 && [ -d node_modules ]; then \
		npx tsc --noEmit; \
	else \
		echo "(node_modules not installed; skipping tsc)"; \
	fi

dev:
	@echo "starting vite dev server on http://localhost:5173"
	@if command -v npx >/dev/null 2>&1; then \
		npx vite; \
	else \
		echo "npx not found; install Node.js + npm first"; \
		exit 1; \
	fi

clean:
	rm -f $(WASM_OUT) $(WASM_EXEC_DST_PUBLIC) $(WASM_EXEC_DST_SRC)
	rm -rf dist
