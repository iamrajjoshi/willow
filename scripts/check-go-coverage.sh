#!/usr/bin/env bash
set -euo pipefail

min_coverage="${GO_COVERAGE_MIN:-70.0}"
profile="${GO_COVERAGE_PROFILE:-coverage.out}"

mkdir -p "$(dirname "$profile")"

go test ./... -count=1 -coverpkg=./... -coverprofile="$profile" -covermode=count

coverage_with_percent="$(go tool cover -func="$profile" | awk '/^total:/ {print $3}')"
coverage="${coverage_with_percent%%%}"
if [[ -z "$coverage" ]]; then
	printf 'Could not determine total Go coverage from %s\n' "$profile" >&2
	exit 1
fi

printf 'Go coverage: %s%%\n' "$coverage"
printf 'Required:    %s%%\n' "$min_coverage"
printf 'Profile:     %s\n' "$profile"

if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
	{
		printf '### Go coverage\n\n'
		printf '| Metric | Value |\n'
		printf '| --- | --- |\n'
		printf '| Coverage | `%s%%` |\n' "$coverage"
		printf '| Required | `%s%%` |\n' "$min_coverage"
		printf '| Profile | `%s` |\n' "$profile"
	} >>"$GITHUB_STEP_SUMMARY"
fi

if ! awk -v coverage="$coverage" -v minimum="$min_coverage" 'BEGIN { exit !(coverage + 0 >= minimum + 0) }'; then
	printf 'Go coverage %s%% is below required %s%%\n' "$coverage" "$min_coverage" >&2
	exit 1
fi
