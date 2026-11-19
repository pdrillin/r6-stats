import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the API comes from `r6rec serve` or `r6server serve`, on
// port 8080 by default; R6_API overrides the target. With r6server, start it
// with -url http://localhost:5173 so that sign-in comes back through here.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": process.env.R6_API ?? "http://127.0.0.1:8080",
      "/auth": process.env.R6_API ?? "http://127.0.0.1:8080",
    },
  },
});
