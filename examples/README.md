# Examples

Files to try cuddio on. Every key, password, address and card number in them is fake: `example.com` addresses, Ofcom drama-range phone numbers, documentation IP ranges and published test card numbers. The AWS keys in `.env.example-leaky` are AWS's own documentation examples, which cuddio leaves alone on purpose.

Run these from this folder:

| File | Try |
|---|---|
| `laravel.log` | `cuddio laravel.log` |
| `nginx-access.log` | `cuddio nginx-access.log --report` |
| `.env.example-leaky` | `cuddio .env.example-leaky` |
| `stacktrace.txt` | `cuddio stacktrace.txt -o stacktrace.clean.txt` |
| `mysql-dump.sql` | `cuddio mysql-dump.sql --mode fake` |
| `postgres-dump.sql` | `cuddio postgres-dump.sql` |
| `sqlite-dump.sql` | `cuddio sqlite-dump.sql --check` |
| `ai-answer.md` | `cuddio laravel.log --map`, then `cuddio restore ai-answer.md` |

`ai-answer.md` is the kind of reply an AI might give after reading the cleaned `laravel.log`. `restore` puts the real values back, and leaves `<EMAIL_9>`, which cuddio never handed out, as it is. `cuddio forget` deletes the stored mapping afterwards.
