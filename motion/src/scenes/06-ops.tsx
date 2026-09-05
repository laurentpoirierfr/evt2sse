import {Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';
import {colors} from '../colors';

const endpoints = [
  ['/ops/liveness', 'processus vivant'],
  ['/ops/readiness', 'PostgreSQL répond (ping)'],
  ['/ops/info', 'version · commit · build'],
];

export default makeScene2D(function* (view) {
  const title = createRef<Txt>();
  const ops = endpoints.map(() => createRef<Rect>());
  const infra = createRef<Rect>();
  const thanks = createRef<Txt>();

  view.add(
    <Rect
      layout
      width="100%"
      height="100%"
      direction="column"
      justifyContent="start"
      alignItems="center"
      gap={48}
      paddingTop={120}
      fill={colors.bg}
    >
      <Rect layout direction="column" alignItems="center" gap={20}>
        <Txt
          ref={title}
          text="Prêt pour Kubernetes"
          fontSize={62}
          fontWeight={700}
          fill={colors.text}
          opacity={0}
        />
        <Rect width={420} height={6} radius={3} fill={colors.cyan} />
      </Rect>

      <Rect layout direction="row" gap={28}>
        {endpoints.map(([path, desc], i) => (
          <Rect
            key={`ops-${i}`}
            ref={ops[i]}
            layout
            direction="column"
            alignItems="center"
            gap={8}
            padding={[20, 30]}
            radius={16}
            fill={colors.panel}
            stroke={colors.cyan}
            lineWidth={2}
            opacity={0}
          >
            <Txt text={path} fontSize={30} fontWeight={700} fill={colors.cyan} />
            <Txt text={desc} fontSize={22} fill={colors.muted} />
          </Rect>
        ))}
      </Rect>

      <Rect
        ref={infra}
        layout
        direction="column"
        alignItems="center"
        gap={8}
        padding={[22, 40]}
        radius={16}
        fill={colors.panel}
        stroke={colors.border}
        lineWidth={2}
        opacity={0}
      >
        <Txt
          text="Image distroless · non-root · multi-arch · version injectée (-ldflags)"
          fontSize={28}
          fill={colors.text}
        />
        <Txt text="Makefile : build · test · lint · image · image-push" fontSize={24} fill={colors.muted} />
      </Rect>

      <Txt
        ref={thanks}
        text="Merci !"
        fontSize={56}
        fontWeight={800}
        fill={colors.green}
        opacity={0}
      />
    </Rect>,
  );

  yield* title().opacity(1, 0.8);
  yield* all(...ops.map((o) => o().opacity(1, 0.5)));
  yield* waitFor(0.3);
  yield* infra().opacity(1, 0.6);
  yield* waitFor(0.4);
  yield* thanks().opacity(1, 0.8);
  yield* waitFor(2.2);
});