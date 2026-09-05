import {defineConfig} from 'vite';
import {createRequire} from 'node:module';

// @motion-canvas/vite-plugin est distribué en CommonJS : import direct possible,
// mais via require on s'affranchit des soucis d'interop ESM.
const require = createRequire(import.meta.url);
const motionCanvas = require('@motion-canvas/vite-plugin').default
  ?? require('@motion-canvas/vite-plugin');
const ffmpeg = require('@motion-canvas/ffmpeg').default
  ?? require('@motion-canvas/ffmpeg');

export default defineConfig({
  plugins: [motionCanvas(), ffmpeg()],
});