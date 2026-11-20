import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In development the API comes from `stratrec serve` or `stratdata serve`, on
// port 8642 by default; STRATDATA_API overrides the target. With stratdata, start it
// with -url http://localhost:5173 so that sign-in comes back through here.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": process.env.STRATDATA_API ?? "http://127.0.0.1:8642",
      "/auth": process.env.STRATDATA_API ?? "http://127.0.0.1:8642",
    },
  },
});
