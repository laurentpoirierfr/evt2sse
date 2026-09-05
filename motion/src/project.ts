import {makeProject} from '@motion-canvas/core';

import title from './scenes/01-title?scene';
import problem from './scenes/02-problem?scene';
import architecture from './scenes/03-architecture?scene';
import channels from './scenes/04-channels?scene';
import resilience from './scenes/05-resilience?scene';
import ops from './scenes/06-ops?scene';

export default makeProject({
  name: 'evt2sse',
  scenes: [title, problem, architecture, channels, resilience, ops],
});