# Clean with --map, "paste" to an AI, restore its answer.
Output docs/gifs/restore.gif
Source tapes/settings.tape
Set Height 720
Source tapes/setup.tape

Type "cuddio laravel.log --map > laravel.clean.log"
Sleep 300ms
Enter
Sleep 1.5s
Type "cat ai-answer.md"
Sleep 300ms
Enter
Sleep 6s
Type "cuddio restore ai-answer.md"
Sleep 300ms
Enter
Sleep 7s
Type "cuddio forget"
Sleep 300ms
Enter
Sleep 2s
