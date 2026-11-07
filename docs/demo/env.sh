export PATH="$PWD/bin:$PATH"
export PS1='\[\e[1;35m\]❯\[\e[0m\] '
cd "$(mktemp -d)" || return
"$OLDPWD/docs/demo/library.sh" playlists

tracks() {
	jq -r '.tracks[] | [.features.bpm, .title, .artist] | @tsv' "$1" |
		awk -F'\t' '{ n = int(($1 - 60) / 5); b = ""; for (i = 0; i < n; i++) b = b "█"; for (; i < 17; i++) b = b " "; printf "%3d %s  %-18s %s\n", $1, b, $2, $3 }'
}
