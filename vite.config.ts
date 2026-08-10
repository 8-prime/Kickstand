import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    // The Go server owns /api. Proxying keeps the browser on one origin, so
    // there is no CORS to configure in development.
    proxy: {
      "/api": {
        target: process.env.BIKETRIP_API ?? "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
