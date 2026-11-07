Output docs/demo/sequence.gif

Set Shell "bash"
Set FontSize 16
Set Width 900
Set Height 540
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 120ms

Hide
Type "source docs/demo/env.sh && tapeit curate --seed 'Lumen Drive' --size 10 --name 'Friday Mix' >/dev/null && clear"
Enter
Sleep 5s
Show

Sleep 1s
Type "tracks playlists/friday-mix.json"
Sleep 300ms
Enter
Sleep 3s
Type "clear"
Enter
Type "tapeit sequence --from playlists/friday-mix.json --flow arc"
Sleep 500ms
Enter
Sleep 2s
Type "tracks playlists/friday-mix.json"
Sleep 300ms
Enter
Sleep 4.5s
