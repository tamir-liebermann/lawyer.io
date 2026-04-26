import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
// Vite config: build output lands in ../static so the Go server
// (which already does http.FileServer(http.Dir("web/static"))) picks it up
// with no further changes. In dev, `npm run dev` runs Vite on :5173 and
// proxies /api/* to the Go server on :8080.
export default defineConfig({
    plugins: [react()],
    build: {
        outDir: '../static',
        emptyOutDir: true,
        sourcemap: true,
    },
    server: {
        port: 5173,
        proxy: {
            '/api': {
                target: 'http://localhost:8080',
                changeOrigin: false,
            },
            '/healthz': {
                target: 'http://localhost:8080',
                changeOrigin: false,
            },
        },
    },
});
