import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the statistics come from `r6rec serve`, on port 8080 by
// default. R6_API overrides the target.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": process.env.R6_API ?? "http://127.0.0.1:8080",
    },
  },
});
