/// <reference types="vite/client" />

/** With this, a build-time setting that is not declared below is a compile error, however it is read. */
interface ViteTypeOptions {
  strictImportMetaEnv: unknown;
}

/** The build-time settings this workspace reads. */
interface ImportMetaEnv {
  /** Where the API is served from, when it is not this page's own origin. */
  readonly VITE_API_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
