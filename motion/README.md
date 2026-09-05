# Présentation Motion Canvas — evt2sse

Vidéo de présentation (français) du projet evt2sse, construite avec
[Motion Canvas](https://motioncanvas.io/) v3.

## Aperçu

![Aperçu animé de la présentation](output/evt2sse.gif)

Vidéo complète (1920×1080, 60 fps, ~33 s) : [`output/evt2sse.mp4`](output/evt2sse.mp4)

## Commandes

```
npm install          # installe les dépendances
npm run serve        # ouvre l'éditeur Motion Canvas (http://localhost:9000)
npm run render       # build de vérification (bundle éditeur dans dist/)
npm run export       # rendu vidéo headless (Chrome + FFmpeg) -> output/evt2sse.mp4
npm run verify       # smoke test : les 6 scènes chargées, aucune erreur runtime
```

## Structure

```
motion/
├── .gitignore
├── index.html
├── package.json             # scripts npm (serve / render / export / verify)
├── package-lock.json
├── tsconfig.json            # jsxImportSource @motion-canvas/2d/lib
├── vite.config.ts           # plugins Motion Canvas + FFmpeg (CJS via createRequire)
├── output/                  # médias générés (non versionnés)
│   ├── evt2sse.gif          # aperçu animé (640×360)
│   └── evt2sse.mp4          # vidéo finale (1920×1080, 60 fps)
├── scripts/
│   ├── export.mjs           # rendu vidéo headless (Chrome + FFmpeg)
│   └── verify.mjs           # smoke test des 6 scènes (Chrome headless)
└── src/
    ├── colors.ts            # palette partagée
    ├── project.ts           # enchaînement des scènes
    ├── project.meta         # réglages éditeur (exporter FFmpeg, 1080p)
    ├── vite-env.d.ts        # déclaration des modules *.scene
    └── scenes/
        ├── 01-title.tsx     # titre
        ├── 02-problem.tsx   # le problème (long polling / polling)
        ├── 03-architecture.tsx # architecture relais / clients
        ├── 04-channels.tsx  # multi-canaux & nomenclature
        ├── 05-resilience.tsx# résilience (reconnexion, replay…)
        ├── 06-ops.tsx       # ops / Kubernetes (liveness, readiness, /ops/info)
        └── *.meta           # réglages par scène (générés par l'éditeur)
```

## Rendu vidéo headless

`npm run export` pilote l'éditeur Motion Canvas avec Chrome headless et
l'exporter **Video (FFmpeg)** (fourni par `@motion-canvas/ffmpeg`). Il produit
`output/evt2sse.mp4` — 1920×1080, 60 fps, ~33 s. La fin de rendu est détectée
quand le fichier mp4 devient lisible par ffprobe (le moov atom étant écrit à la
fin).

Le GIF `output/evt2sse.gif` est généré depuis la vidéo avec FFmpeg :

```
ffmpeg -i output/evt2sse.mp4 -vf "fps=12,scale=640:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" output/evt2sse.gif
```

Prerequis des scripts : Chrome installé (`/usr/bin/google-chrome`).
