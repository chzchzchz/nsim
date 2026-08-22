import { defineConfig } from "vite";

export default defineConfig({
    root: ".",
    publicDir: "public",
    server: {
        port: 5173,
        strictPort: true,
    },
    build: {
        outDir: "dist",
        target: "es2020",
    },
});
