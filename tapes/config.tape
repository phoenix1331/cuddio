# A project .cuddio.yaml adds a custom rule and an allowlist entry.
Output docs/gifs/config.gif
Source tapes/settings.tape
Set Height 520
Source tapes/setup.tape

Type "grep -E 'HOST|FROM' .env.example-leaky | cuddio"
Sleep 300ms
Enter
Sleep 3.5s
Type "cat > .cuddio.yaml <<'EOF'"
Enter
Type "allowlist:"
Enter
Type "  values: [hello@example.com]"
Enter
Type "rules:"
Enter
Type "  custom:"
Enter
Type "    - id: internal-host"
Enter
Type "      category: secret"
Enter
Type "      label: HOST"
Enter
Type `      pattern: '[a-z]+\.internal\.example\.com'`
Enter
Type "EOF"
Enter
Sleep 1s
Type "grep -E 'HOST|FROM' .env.example-leaky | cuddio"
Sleep 300ms
Enter
Sleep 5s
