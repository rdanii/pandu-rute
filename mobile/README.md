# mobile/ — not yet scaffolded

This directory is a placeholder. The Expo app has **not** been created yet
because Node.js was not installed on the machine where the repo was bootstrapped.

## To scaffold

1. Install Node.js LTS (includes `npm`/`npx`).
2. From the repo root, remove this placeholder and run the official tool:

   ```bash
   rm mobile/README.md && rmdir mobile
   npx create-expo-app@latest mobile --template blank-typescript
   ```

   (`create-expo-app` refuses to write into a non-empty directory, hence the removal.)
3. Validate:

   ```bash
   cd mobile
   npx expo-doctor
   npx tsc --noEmit
   npx expo start   # then open in Expo Go or an emulator
   ```

No product features should be added as part of the scaffold.
