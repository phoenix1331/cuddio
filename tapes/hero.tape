# A Laravel log before and after cuddio: emails, phone, card, IP and
# tokens become placeholders the AI can still tell apart.
Output docs/gifs/hero.gif
Source tapes/settings.tape
Set Height 380
Source tapes/setup.tape

Type "sed -n '7,8p' laravel.log"
Sleep 300ms
Enter
Sleep 3.5s
Type "sed -n '7,8p' laravel.log | cuddio"
Sleep 300ms
Enter
Sleep 6s
