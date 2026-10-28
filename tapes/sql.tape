# Column-aware SQL cleaning: whole values in sensitive columns, fake mode
# for a dump that still imports, then --check.
Output docs/gifs/sql.gif
Source tapes/settings.tape
Set Height 380
Source tapes/setup.tape

Type "grep 'INSERT INTO `users`' mysql-dump.sql | cut -c1-118"
Sleep 300ms
Enter
Sleep 3s
Type "cuddio mysql-dump.sql | grep 'INSERT INTO `users`' | cut -c1-118"
Sleep 300ms
Enter
Sleep 3.5s
Type "cuddio mysql-dump.sql --mode fake | grep 'INSERT INTO `users`' | cut -c1-118"
Sleep 300ms
Enter
Sleep 4s
Type "cuddio mysql-dump.sql --check"
Sleep 300ms
Enter
Sleep 3.5s
