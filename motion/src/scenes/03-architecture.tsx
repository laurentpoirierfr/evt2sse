import {Rect, Line, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';
import {colors} from '../colors';

// Connexions SSE relay → clients (paires de points [x, y]).
const clientArcs: Array<[[number, number], [number, number]]> = [
  [[163, -270], [620, -270]],
  [[163, -40], [620, -40]],
  [[163, 190], [620, 190]],
];

export default makeScene2D(function* (view) {
  const title = createRef<Txt>();
  const producer = createRef<Rect>();
  const pg = createRef<Rect>();
  const relay = createRef<Rect>();
  const clientA = createRef<Rect>();
  const clientB = createRef<Rect>();
  const clientC = createRef<Rect>();
  const arc = createRef<Line>();
  const arcListen = createRef<Line>();
  const arcs = [0, 1, 2].map(() => createRef<Line>());
  const labelNotify = createRef<Txt>();
  const labelListen = createRef<Txt>();
  const labelSse = createRef<Txt>();

  view.add(
    <Rect width="100%" height="100%" fill={colors.bg}>
      <Txt
        ref={title}
        x={0}
        y={-430}
        text="Architecture"
        fontSize={56}
        fontWeight={700}
        fill={colors.text}
        opacity={0}
      />

      <Rect
        ref={producer}
        x={-560}
        y={-250}
        layout
        direction="column"
        alignItems="center"
        justifyContent="center"
        padding={[18, 30]}
        fill={colors.panel}
        stroke={colors.orange}
        lineWidth={3}
        radius={16}
        opacity={0}
      >
        <Txt text="Producteur" fontSize={34} fontWeight={700} fill={colors.text} />
        <Txt text="API / trigger SQL" fontSize={24} fill={colors.muted} />
      </Rect>
      <Rect
        ref={pg}
        x={-560}
        y={-40}
        layout
        direction="column"
        alignItems="center"
        justifyContent="center"
        padding={[18, 30]}
        fill={colors.panel}
        stroke={colors.postgres}
        lineWidth={3}
        radius={16}
        opacity={0}
      >
        <Txt text="PostgreSQL" fontSize={34} fontWeight={700} fill={colors.text} />
        <Txt text="NOTIFY / LISTEN" fontSize={24} fill={colors.muted} />
      </Rect>
      <Rect
        ref={relay}
        x={60}
        y={-40}
        layout
        direction="column"
        alignItems="center"
        justifyContent="center"
        padding={[18, 30]}
        fill={colors.panel}
        stroke={colors.cyan}
        lineWidth={3}
        radius={16}
        opacity={0}
      >
        <Txt text="evt2sse (Go)" fontSize={34} fontWeight={700} fill={colors.text} />
        <Txt text="relay + SSE" fontSize={24} fill={colors.muted} />
      </Rect>
      <Rect
        ref={clientA}
        x={760}
        y={-270}
        layout
        direction="column"
        alignItems="center"
        justifyContent="center"
        padding={[18, 30]}
        fill={colors.panel}
        stroke={colors.green}
        lineWidth={3}
        radius={16}
        opacity={0}
      >
        <Txt text="Navigateur" fontSize={32} fontWeight={700} fill={colors.text} />
        <Txt text="UI / SSE" fontSize={22} fill={colors.muted} />
      </Rect>
      <Rect
        ref={clientB}
        x={760}
        y={-40}
        layout
        direction="column"
        alignItems="center"
        justifyContent="center"
        padding={[18, 30]}
        fill={colors.panel}
        stroke={colors.green}
        lineWidth={3}
        radius={16}
        opacity={0}
      >
        <Txt text="Client Go" fontSize={32} fontWeight={700} fill={colors.text} />
        <Txt text="pkg/client" fontSize={22} fill={colors.muted} />
      </Rect>
      <Rect
        ref={clientC}
        x={760}
        y={190}
        layout
        direction="column"
        alignItems="center"
        justifyContent="center"
        padding={[18, 30]}
        fill={colors.panel}
        stroke={colors.green}
        lineWidth={3}
        radius={16}
        opacity={0}
      >
        <Txt text="curl / scripts" fontSize={32} fontWeight={700} fill={colors.text} />
        <Txt text="SSE" fontSize={22} fill={colors.muted} />
      </Rect>

      <Line
        ref={arc}
        points={[[-560, -175], [-560, -105]]}
        stroke={colors.orange}
        lineWidth={4}
        endArrow
        opacity={0}
      />
      <Txt
        ref={labelNotify}
        x={-430}
        y={-150}
        text="pg_notify(channel, payload)"
        fontSize={22}
        fill={colors.orange}
        opacity={0}
      />
      <Line
        ref={arcListen}
        points={[[-485, -40], [-180, -40]]}
        stroke={colors.postgres}
        lineWidth={4}
        endArrow
        opacity={0}
      />
      <Txt
        ref={labelListen}
        x={-330}
        y={-75}
        text="LISTEN (1 connexion / canal)"
        fontSize={20}
        fill={colors.postgres}
        opacity={0}
      />
      <Txt
        ref={labelSse}
        x={300}
        y={-160}
        text="SSE /api/listen"
        fontSize={24}
        fill={colors.green}
        opacity={0}
      />

      {clientArcs.map((points, i) => (
        <Line
          key={`arc-${i}`}
          ref={arcs[i]}
          points={points}
          stroke={colors.green}
          lineWidth={4}
          endArrow
          opacity={0}
        />
      ))}
    </Rect>,
  );

  yield* title().opacity(1, 0.8);
  yield* all(
    producer().opacity(1, 0.6),
    pg().opacity(1, 0.6),
    relay().opacity(1, 0.6),
  );
  yield* all(arc().opacity(1, 0.5), labelNotify().opacity(1, 0.5));
  yield* all(arcListen().opacity(1, 0.5), labelListen().opacity(1, 0.5));
  yield* all(
    clientA().opacity(1, 0.5),
    clientB().opacity(1, 0.5),
    clientC().opacity(1, 0.5),
    labelSse().opacity(1, 0.5),
  );
  yield* all(...arcs.map((a) => a().opacity(1, 0.5)));
  yield* waitFor(2.2);
});