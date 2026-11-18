// vite empties the output folder before it writes; the placeholder awkit's
// own build needs is put back.
import { writeFileSync } from "node:fs";
writeFileSync(new URL("../../internal/workbench/dist/.keep", import.meta.url), "");
