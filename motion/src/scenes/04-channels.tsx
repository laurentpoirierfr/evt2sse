import {Rect, Txt, makeScene2D} from '@motion-canvas/2d';
import {all, createRef, waitFor} from '@motion-canvas/core';
import {colors} from '../colors';

const channels = [
  ['orders.created', colors.green],
  ['orders.shipped', colors.cyan],
  ['inventory.low', colors.orange],
];

export default makeScene2D(function* (view) {
  const title = createRef<Txt>();
  const formula = createRef<Rect>();
  const pills = channels.map(() => createRef<Rect>());
  const note = createRef<Txt>();

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
          text="Multi-canaux & nomenclature"
          fontSize={60}
          fontWeight={700}
          fill={colors.text}
          opacity={0}
        />
        <Rect width={420} height={6} radius={3} fill={colors.cyan} />
      </Rect>
      <Rect
        ref={formula}
        layout
        direction="row"
        gap={14}
        padding={[14, 26]}
        radius={14}
        fill={colors.panel}
        stroke={colors.border}
        lineWidth={2}
        opacity={0}
      >
        <Txt text="{" fontSize={26} fill={colors.muted} />
        <Txt text="domaine" fontSize={26} fill={colors.green} />
        <Txt text="." fontSize={26} fill={colors.muted} />
        <Txt text="contexte" fontSize={26} fill={colors.cyan} />
        <Txt text="." fontSize={26} fill={colors.muted} />
        <Txt text="événement" fontSize={26} fill={colors.orange} />
        <Txt text="}" fontSize={26} fill={colors.muted} />
      </Rect>

      <Rect layout direction="row" gap={30}>
        {channels.map(([name, color], i) => (
          <Rect
            key={`pill-${i}`}
            ref={pills[i]}
            layout
            padding={[18, 34]}
            radius={40}
            fill={colors.panel}
            stroke={color}
            lineWidth={3}
            opacity={0}
          >
            <Txt text={name} fontSize={34} fontWeight={700} fill={colors.text} />
          </Rect>
        ))}
      </Rect>

      <Txt
        ref={note}
        layout
        text="POST /api/channels · une connexion LISTEN dédiée par canal"
        fontSize={28}
        fill={colors.muted}
        opacity={0}
      />
    </Rect>,
  );

  yield* title().opacity(1, 0.8);
  yield* formula().opacity(1, 0.6);
  yield* all(...pills.map((p) => p().opacity(1, 0.6)));
  yield* waitFor(0.4);
  yield* note().opacity(1, 0.6);
  yield* waitFor(2);
});