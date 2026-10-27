There are two separate problems in that log.

**1. The database login is failing.** MySQL rejects the `shop` user connecting from `<IP_1>`, so the lookup for `<EMAIL_1>` never runs. Check that `DB_PASSWORD` on that host matches the MySQL user, and that `shop` is allowed to connect from `<IP_1>`.

**2. Mail can't be sent.** The SMTP connection string has the password `<PASSWORD_1>` written into it. Move it into `MAIL_PASSWORD` and rotate it, since it has been in your logs. The failed message was going to `<EMAIL_3>`.

The checkout itself looks fine: `<EMAIL_2>` started it from `<IP_2>` with card `<CARD_1>`. If the order is stuck, call them on `<PHONE_1>`.

I couldn't find `<EMAIL_9>` in what you sent, so I've left it as it is.
