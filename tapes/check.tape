# --check and --report in a CI-style run: counts only, never the values,
# and exit 1 when anything would be hidden.
Output docs/gifs/check.gif
Source tapes/settings.tape
Set Height 420
Source tapes/setup.tape

Type `cuddio --check nginx-access.log; echo "exit code $?"`
Sleep 300ms
Enter
Sleep 3s
Type "cuddio --report nginx-access.log"
Sleep 300ms
Enter
Sleep 4s
Type `cuddio --check README.md; echo "exit code $?"`
Sleep 300ms
Enter
Sleep 3.5s
