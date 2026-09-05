/// <reference types="vite/client" />

declare module '*?scene' {
  const value: import('@motion-canvas/core').FullSceneDescription;
  export = value;
}