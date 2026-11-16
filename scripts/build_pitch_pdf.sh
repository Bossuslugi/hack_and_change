#!/usr/bin/env bash
# Собирает Avito_Start_AI_Buddy_Presentation.pdf из docs/presentation/pitch_deck.html.
# Вся логика — в build_pitch_pdf.mjs (git remote → QR → headless Edge/Chrome → PDF).
set -e
cd "$(dirname "$0")/.."
node scripts/build_pitch_pdf.mjs "$@"
