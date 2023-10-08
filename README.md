# Mail Server
Server for handling the Nintendo Wii's message system.

## How to use
You will need:
- PostgreSQL
- Mailgun (20k free message per month with Github Student Pack)
- Sentry (Error logging)

Copy `config-example.xml` to `config.xml` and insert all the correct data.

Next you will need to patch your Wii to point to your domain. Editing all instances of `wiilink24.com` in our [Mail Patcher](https://github.com/WiiLink24/Mail-Patcher), then compiling will do that for you.

To get a Wii to actually request to your servers, you will need to proxy your domain. Consider something like Cloudflare.

Finally, `go build` and run the executable!

## Future plans:
- Open source API for dispatching messages across message boards.
  - Creators will be able to have their own "community" to which users can subscribe to.
  - Creators can then upload messages and send to subscribed message boards.

- Direct integration with Dolphin Emulator

