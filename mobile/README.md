# mobile/

React Native + Expo app (TypeScript), scaffolded from the `blank-typescript`
template. `App.tsx` now contains an Indonesian home screen:

- app name "Pandu Rute" and the tagline "Biar nggak bingung di jalan.";
- one free-text destination field ("Mau ke mana?") and a "Cari Rute" button;
- validation of an empty destination, and a notice after submitting.

Current limitations:

- **No backend integration.** Submitting only records the typed destination and
  says that route search is not available yet. The app never calls the Go API.
- There is no origin field, no stop search and no location permission. The typed
  text is not geocoded and is not a location.
- No dependencies were added beyond the template.

## Versions (from package.json)

Expo SDK `~57.0.27`, React Native `0.86.3`, React `19.2.3`, TypeScript `~6.0.3`.
Scaffolded and validated with Node.js v24.21.0 / npm 11.19.0 (via nvm).

## Setup

```bash
cd mobile
npm install
```

## Validate

```bash
npx expo-doctor                 # project health checks
npx tsc --noEmit                # type check (tsconfig extends expo/tsconfig.base, strict)
npx expo install --check        # deps match the Expo SDK
npx expo export --platform android --output-dir /tmp/expo-export   # bundles the JS
```

## Run

```bash
npx expo start
```

On WSL2, Expo Go on a phone may not reach the dev server over the LAN; use
`npx expo start --tunnel` (the `@expo/ngrok` package must be installed).

The home screen was opened in Expo Go on a physical iPhone over `--tunnel`,
and its layout, empty-input validation, submission notice and keyboard
behaviour were checked manually by the repository owner; this was a manual
check, not an automated test. Android and screen readers have not been checked.

## How this was scaffolded

```bash
npx create-expo-app@latest mobile --template blank-typescript --no-install --no-agents-md
```

The template was generated in an empty temporary directory and copied in
(this README already existed). Two template outputs were deliberately left out:

- its own nested `.git/` repository, since `mobile/` belongs to the monorepo;
- its `LICENSE` file (MIT, © 650 Industries / Expo). A licence for this
  project has not been chosen yet.

`--no-agents-md` skips the AI-assistant files (`AGENTS.md`, `.claude/settings.json`)
that `create-expo-app` now generates by default.

## Known issues

`npm audit` reports 22 vulnerabilities (7 moderate, 15 high), all pulled in
transitively by `expo` and `react-native` at the versions the SDK pins.
`npm audit fix --force` would break SDK alignment; prefer upgrading the Expo
SDK when patched versions are available.
