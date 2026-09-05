import {Circle, Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';
import {colors} from '../colors';

const bullets = [
  'Reconnexion automatique — backoff + jitter (plafond 30 s)',
  'Reprise Last-Event-ID après coupure (tampon 256 événements)',
  "Envoi idempotent : id dédupliqué 5 min côté relais",
  'Heartbeat SSE pour détecter les clientes coupées',
  'Démarrage tolérant à une base indisponible (retry)',
];

export default makeScene2D(function* (view) {
  const title = createRef<Txt>();
  const rows = bullets.map(() => createRef<Rect>());
  const checks = bullets.map(() => createRef<Circle>());

  view.add(
    <Rect
      layout
      width="100%"
      height="100%"
      direction="column"
      justifyContent="start"
      alignItems="center"
      gap={60}
      paddingTop={120}
      fill={colors.bg}
    >
      <Rect layout direction="column" alignItems="center" gap={20}>
        <Txt
          ref={title}
          text="Pensé pour la résilience"
          fontSize={60}
          fontWeight={700}
          fill={colors.text}
          opacity={0}
        />
        <Rect width={420} height={6} radius={3} fill={colors.cyan} />
      </Rect>
      <Rect layout direction="column" gap={32}>
        {bullets.map((b, i) => (
          <Rect
            key={`row-${i}`}
            ref={rows[i]}
            layout
            direction="row"
            alignItems="center"
            gap={24}
            x={50}
            opacity={0}
          >
            <Circle ref={checks[i]} width={30} height={30} fill={colors.green} scale={0} />
            <Txt text={b} fontSize={34} fill={colors.text} />
          </Rect>
        ))}
      </Rect>
    </Rect>,
  );

  yield* title().opacity(1, 0.8);
  for (let i = 0; i < rows.length; i++) {
    yield* all(
      rows[i]().x(0, 0.55),
      rows[i]().opacity(1, 0.55),
      checks[i]().scale(1, 0.55),
    );
    yield* waitFor(0.3);
  }
  yield* waitFor(1.8);
});