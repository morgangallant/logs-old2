[![Deploy on Railway](https://railway.app/button.svg)](https://railway.app/new/template?template=https%3A%2F%2Fgithub.com%2Fmorgangallant%2Flogs&plugins=postgresql&envs=TELEGRAM_USERNAME%2CTELEGRAM_SECRET%2COWNER_NAME&optionalEnvs=OWNER_NAME&TELEGRAM_USERNAMEDesc=Your+telegram+username.&TELEGRAM_SECRETDesc=The+generated+telegram+secret.&OWNER_NAMEDesc=Your+name%2C+will+be+displayed+on+website.&OWNER_NAMEDefault=John+Doe)

This is the underlying implemention behind my [Logs Infrastructure](https://logs.morgangallant.com). Using the Deploy on Railway button, you can easily setup your own logs server (for free).

Steps to Setup:
- Deploy on Railway, fill in your Telegram username, and do CMD+K -> Generate Secret to generate Telegram Secret. Owner Name is your name.
- Make a new Telegram bot w/ Botfather.
- Open browser, do request to https://api.telegram.org/botBOTFATHERKEY/setWebhook?url=https://DOMAIN/_wh/telegram?key=GENERATED_SECRET (replacing values).
- That should be it? idk good luck.
