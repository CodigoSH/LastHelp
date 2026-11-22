# Search engines

The public boards are meant to be found; what is behind a session is not. This page says what LastHelp tells a search engine, so you can check it against your own instance.

## What is public and what is not

A published organization, board and roadmap can be indexed. These cannot, and answer with `X-Robots-Tag: noindex, nofollow` and the same instruction in the page itself:

- the panel, the team's archive at `/review`, and the sign-in and setup screens;
- any address that carries a follow token (`?k=`), because that link is the key to somebody's own report;
- everything under `/api/`.

A private organization or board is not in a search at all: to anybody who is not on the team its address is a 404, as if it did not exist.

`robots.txt` forbids none of that, on purpose. A crawler that is not allowed to fetch an address never reads the `noindex` on it, and an address that is linked from somewhere else can then be listed anyway, without its page.

## The sitemap

`/sitemap.xml` lists the front page, each public organization and each public board with its roadmap, and `robots.txt` points at it. It is the same file for everybody: a private board is not in it even when an administrator asks.

It needs `LASTHELP_PUBLIC_URL`, because a sitemap carries absolute addresses and the only trustworthy one is the one you configured. With none set there is no sitemap (`/sitemap.xml` answers 404) and `robots.txt` does not mention one.

Give the address to the search engines yourself, in their own webmaster tools; nothing here does it for you.

## An address that does not exist

LastHelp is one page that draws its own screens, so an address with no screen used to answer 200 like any other. It now answers **404**, with the same page, which draws the *Not found* message. A published board, its roadmap and its report form are 200; so are the panel, `/review` and the sign-in screens, whether or not what is behind them exists, because those are kept out of search on their own terms.

If a proxy of yours replaces error pages (`proxy_intercept_errors` in nginx, `handle_errors` in Caddy), it may replace this one too. The status is what tells a search engine the address is not a page; the body is the same application page either way.
