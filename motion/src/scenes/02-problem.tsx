import {Circle, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';
import {colors} from '../colors';

const bullets: Array<[string, string, string]> = [
  ['Polling HTTP', 'inefficace, latence, charge inutile', colors.orange],
  ['WebSockets', 'état permanent, proxies, repli compliqué', colors.red],
  ['Message broker', 'infrastructure lourde pour un simple pub/sub', colors.violet],
];

export default makeScene2D(function* (view) {
  const title = createRef<Txt>();
  const rows = bullets.map(() => createRef<Rect>());
  const dots = bullets.map(() => createRef<Circle>());

  view.add(
    <Rect
      layout
      width="100%"
      height="100%"
      direction="column"
      justifyContent="start"
      alignItems="center"
      gap={64}
      paddingTop={120}
      fill={colors.bg}
    >
      <Rect layout direction="column" alignItems="center" gap={20}>
        <Txt
          ref={title}
          text="Distribuer des événements en temps réel"
          fontSize={60}
          fontWeight={700}
          fill={colors.text}
          opacity={0}
        />
        <Rect width={420} height={6} radius={3} fill={colors.cyan} />
      </Rect>
      <Rect layout direction="column" gap={44}>
        {bullets.map(([name, desc, color], i) => (
          <Rect
            key={`row-${i}`}
            ref={rows[i]}
            layout
            direction="row"
            alignItems="center"
            gap={26}
            x={40}
            opacity={0}
          >
            <Circle ref={dots[i]} width={26} height={26} fill={color} scale={0} />
            <Rect layout direction="column" gap={4}>
              <Txt text={name} fontSize={42} fontWeight={700} fill={colors.text} />
              <Txt text={desc} fontSize={30} fill={colors.muted} />
            </Rect>
          </Rect>
        ))}
      </Rect>
    </Rect>,
  );

  yield* all(title().opacity(1, 0.9));
  for (let i = 0; i < rows.length; i++) {
    yield* all(
      rows[i]().x(0, 0.6),
      rows[i]().opacity(1, 0.6),
      dots[i]().scale(1, 0.6),
    );
    yield* waitFor(0.35);
  }
  yield* waitFor(1.6);
});