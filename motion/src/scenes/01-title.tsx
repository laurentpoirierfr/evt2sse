import {Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';
import {colors} from '../colors';

export default makeScene2D(function* (view) {
  const title = createRef<Txt>();
  const sub = createRef<Txt>();
  const badge = createRef<Rect>();

  view.add(
    <Rect
      layout
      width="100%"
      height="100%"
      direction="column"
      justifyContent="center"
      alignItems="center"
      gap={28}
      fill={colors.bg}
    >
      <Txt
        ref={title}
        text="evt2sse"
        fontSize={180}
        fontWeight={900}
        fill={colors.cyan}
        scale={0.85}
        opacity={0}
      />
      <Txt
        ref={sub}
        text="Événements PostgreSQL → Server-Sent Events"
        fontSize={46}
        fill={colors.muted}
        opacity={0}
      />
      <Rect
        ref={badge}
        layout
        padding={[14, 28]}
        radius={24}
        fill={colors.panel}
        stroke={colors.border}
        lineWidth={2}
        opacity={0}
      >
        <Txt
          text="Go · NOTIFY / LISTEN · SSE · Docker/K8s"
          fontSize={28}
          fill={colors.text}
        />
      </Rect>
    </Rect>,
  );

  yield* all(title().opacity(1, 1), title().scale(1, 1));
  yield* sub().opacity(1, 0.9);
  yield* badge().opacity(1, 0.7);
  yield* waitFor(2);
});