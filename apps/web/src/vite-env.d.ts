/// <reference types="vite/client" />

declare module "@fontsource-variable/inter" {
  const content: any;
  export default content;
}

declare module "@fontsource-variable/manrope" {
  const content: any;
  export default content;
}

interface ImportMetaEnv {
  readonly VITE_CONTROL_PLANE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
