#! /bin/bash
set -e

printf "Running gofmt checks...\n"
OUTPUT=$(gofmt -d -s .)

if [[ $OUTPUT ]]; then
  printf "gofmt found unformatted files:\n\n"
  echo "$OUTPUT"
  exit 1
fi
