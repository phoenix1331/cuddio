# cuddio --clip cleans whatever is on the clipboard in place. copy and
# paste are hidden helpers for the Windows clipboard from WSL.
Output docs/gifs/clipboard.gif
Source tapes/settings.tape
Set Height 420
Source tapes/setup.tape

Hide
Type `copy() { powershell.exe -NoProfile -Command '[Console]::InputEncoding=[Text.Encoding]::UTF8; Set-Clipboard -Value ([Console]::In.ReadToEnd())'; }`
Enter
Type `paste() { powershell.exe -NoProfile -Command '[Console]::OutputEncoding=[Text.Encoding]::UTF8; $c=Get-Clipboard -Raw; if ($c -ne $null) { [Console]::Out.Write($c) }'; }`
Enter
Type "sed -n '1,2p' stacktrace.txt | copy && clear"
Enter
Sleep 3s
Show

Type "paste"
Sleep 300ms
Enter
Sleep 3.5s
Type "cuddio --clip"
Sleep 300ms
Enter
Sleep 3s
Type "paste"
Sleep 300ms
Enter
Sleep 4.5s
