import { defineConfig } from "vite";

export default defineConfig({
    root: ".",
    // Relative asset URLs so a production bundle serves unchanged
    // from any mount point, not only the domain root.
    base: "./",
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
