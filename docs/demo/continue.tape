Output docs/demo/continue.gif

Set Shell "bash"
Set FontSize 16
Set Width 900
Set Height 540
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 120ms

Hide
Type "source docs/demo/env.sh && clear"
Enter
Sleep 5s
Show

Sleep 1s
Type "tracks playlists/sunday-soul.json | head -6"
Sleep 300ms
Enter
Sleep 2.5s
Type "tapeit curate --seed-playlist sunday-soul --size 10"
Sleep 500ms
Enter
Sleep 2.5s
Type "tracks playlists/more-like-sunday-soul.json"
Sleep 300ms
Enter
Sleep 4s
