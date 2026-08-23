# nsim-web — Makefile
#
# Targets:
#   make build   — compile wasm, copy polyfill, run tsc
#   make wasm    — compile wasm + copy wasm_exec.js into public/
#   make wasm-exec — copy $(go env GOROOT)/lib/wasm/wasm_exec.js into public/ and src/
#   make typecheck — tsc --noEmit
#   make dev     — vite dev server on :5173
#   make bundle  — pack web.tgz (release: minified JS, stripped
#                  wasm) which extracts to nsim-web/ ready for
#                  static hosting; alias for bundle-release
#   make bundle-debug — pack web-debug.tgz (readable JS with
#                  sourcemaps, wasm keeps full debug info)
#   make clean   — remove public/nsim.wasm, dist/, src/wasm_exec.js,
#                  web.tgz, web-debug.tgz

GO ?= go
GOROOT := $(shell $(GO) env GOROOT)
WASM_SRC := wasm
WASM_OUT := public/nsim.wasm
WASM_EXEC_DST_PUBLIC := public/wasm_exec.js
WASM_EXEC_DST_SRC := src/wasm_exec.js
BUNDLE_DIR := nsim-web
BUNDLE_TGZ := web.tgz
BUNDLE_DEBUG_TGZ := web-debug.tgz
BUNDLE_DEPS := $(shell find src index.html vite.config.ts package.json 2>/dev/null)

.PHONY: all build wasm wasm-exec typecheck dev bundle bundle-release bundle-debug clean

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

# Release packs minified JS and a symbol-stripped wasm binary;
# debug packs readable JS plus sourcemaps and the unstripped wasm.
# Target-specific variables drive both variants through one recipe:
# vite build is not file-trackable per output, so the tarballs just
# depend directly on every front-end source and the wasm blob.
web.tgz web-debug.tgz: $(WASM_EXEC_DST_PUBLIC) $(WASM_OUT) $(BUNDLE_DEPS)
	@if [ ! -d node_modules ]; then \
		echo "node_modules not installed; run 'npm install' first" >&2; \
		exit 1; \
	fi
	npx vite build $(VITE_FLAGS) --outDir $(DIST_DIR)
	rm -rf .bundle && mkdir -p .bundle/$(BUNDLE_DIR)
	cp -r $(DIST_DIR)/. .bundle/$(BUNDLE_DIR)/
	if [ "$(VARIANT)" = "release" ]; then \
		GOOS=js GOARCH=wasm $(GO) build -ldflags="all=-s -w" -trimpath \
			-o .bundle/$(BUNDLE_DIR)/nsim.wasm ./$(WASM_SRC); \
	fi
	tar czf $@ -C .bundle $(BUNDLE_DIR)
	rm -rf .bundle
	@echo "wrote $@ ($(VARIANT)); extract to serve ./$(BUNDLE_DIR)/ as static content"

web.tgz: VARIANT := release
web.tgz: DIST_DIR := dist
web.tgz: VITE_FLAGS :=
web-debug.tgz: VARIANT := debug
web-debug.tgz: DIST_DIR := dist-debug
web-debug.tgz: VITE_FLAGS := --minify false --sourcemap

bundle: web.tgz
bundle-release: web.tgz
bundle-debug: web-debug.tgz

clean:
	rm -f $(WASM_OUT) $(WASM_EXEC_DST_PUBLIC) $(WASM_EXEC_DST_SRC) $(BUNDLE_TGZ) $(BUNDLE_DEBUG_TGZ)
	rm -rf dist dist-debug .bundle
