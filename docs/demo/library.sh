#!/usr/bin/env bash
set -euo pipefail

dir="${1:?usage: library.sh DIR}"
mkdir -p "$dir"

indie=("The Paper Lanterns" "Velvet Static" "Harbor Lights" "Neon Orchard" "Cold Radio" "Kite Season" "The Lowtide Club" "Static Bloom")
electro=("Midnight Circuit" "Lumen Drive" "Pulse Theory" "Glasswave" "Sundial" "Night Ferry" "Copper Moon" "Halcyon Grid")
soul=("Ada Monroe" "The Brass Hours" "Lou Carver Trio" "Marigold Soul" "Ezra Bloom" "Velma Rhodes" "The Sunday Choir" "Otis Fairweather")
adj=("Golden" "Paper" "Northern" "Silver" "Electric" "Quiet" "Midnight" "Velvet")
noun=("Hour" "Hearts" "Lights" "Avenue" "Rain" "Signals" "Harbor" "Summer" "Echoes" "Fever" "Satellite" "Weather")

song() {
	local cluster=$1 a=$2 s=$3
	local n=$((((cluster * 32 + a * 4 + s) * 41) % 96))
	printf '%s %s' "${adj[$((n / 12))]}" "${noun[$((n % 12))]}"
}

bpm() {
	local cluster=$1 a=$2 s=$3
	local base=(112 122 84)
	echo $((base[cluster] + (a * 11 + s * 17) % 41 - 20))
}

artist() {
	local cluster=$1 a=$2
	case $cluster in
	0) printf '%s' "${indie[$a]}" ;;
	1) printf '%s' "${electro[$a]}" ;;
	2) printf '%s' "${soul[$a]}" ;;
	esac
}

track() {
	local cluster=$1 a=$2 s=$3
	printf '    {"title": "%s", "artist": "%s", "features": {"bpm": %d}}' \
		"$(song "$cluster" "$a" "$s")" "$(artist "$cluster" "$a")" "$(bpm "$cluster" "$a" "$s")"
}

playlist() {
	local slug=$1 name=$2 cluster=$3 shift=$4 cross=$5
	{
		printf '{\n  "name": "%s",\n  "tracks": [\n' "$name"
		local first=1 i
		for i in $(seq 0 13); do
			local a=$(((i + shift) % 8)) s=$(((i / 8 + shift) % 4))
			[ $first -eq 1 ] || printf ',\n'
			first=0
			track "$cluster" "$a" "$s"
		done
		if [ "$cross" -ge 0 ]; then
			printf ',\n'
			track "$cross" "$shift" 0
		fi
		printf '\n  ]\n}\n'
	} >"$dir/$slug.json"
}

playlist indie-mornings "Indie Mornings" 0 0 -1
playlist garage-nights "Garage Nights" 0 3 -1
playlist bedroom-pop "Bedroom Pop" 0 5 1
playlist road-trip "Road Trip" 0 1 2
playlist late-night-drive "Late Night Drive" 1 0 0
playlist warehouse "Warehouse" 1 2 -1
playlist deep-focus "Deep Focus" 1 4 -1
playlist neon-city "Neon City" 1 6 0
playlist sunday-soul "Sunday Soul" 2 0 -1
playlist dinner-party "Dinner Party" 2 3 0
playlist slow-burn "Slow Burn" 2 5 -1
playlist vinyl-crackle "Vinyl Crackle" 2 7 1
