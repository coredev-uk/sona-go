# Apple Music web app reference

This is the reference inventory Sona's features follow. It lists every page and action of the current Apple Music web app (beta.music.apple.com, the same app that serves music.apple.com). For each one it gives the data it shows, the actions it offers, its route, and the exact API calls behind it.

**It covers functionality only.** Apple's layout, visual design, wording and styling are not a reference. Each Sona page is designed fresh under `docs/design.md` and the design skills (`skill://impeccable`, `skill://design-taste-frontend`). Sona should match Apple in capability, not in appearance. Shelf titles quoted below are data that Apple's API returns, not copy to reuse.

Examples use `{sf}` for the storefront; observed with a subscriber account.

## How this was produced

- **Observed.** A headless browser profile, signed in with a Music User Token (MUT), browsed every page and action. Every `amp-api*`, `api.music`, `play.*` and activity request was recorded with its method, full query and body. Tokens were never recorded.
- **Bundle-read.** For actions that weren't driven through the UI, the request shape comes from the web app's own JS bundle (`/assets/index~*.js` and its lazy chunks). These are marked `[bundle]`.
- **Live-verified.** Each endpoint below was then replayed through `internal/applemusic.Client.Do` with the web player developer token, `Origin: https://music.apple.com`, and the MUT (or no MUT, to test whether it is required). Its status and a trimmed response shape are recorded here.
- **Mutations** were verified as round trips that were reverted afterwards (a temporary playlist created, edited, pinned and deleted; a song rated, favourited, added to the library and removed again).
- `[INFERENCE]` marks anything that was concluded rather than observed.

Status legend: `200`/`202`/`204` = the HTTP status observed. `2xx` = success with an empty body (`Client.Do` hides the exact 2xx code). `MUT: yes` = 401/403/404 without the Music User Token. `MUT: no` = 200 without it.

## 1. Conventions shared by every call

### Hosts

| Host | Used for | Notes |
|---|---|---|
| `amp-api.music.apple.com` | Almost everything: catalog, library, `/v1/me/*`, ratings, favourites, stations, social, lyrics | The web app's main host. It accepts `platform=web` and returns the web-only attributes and relationships. |
| `amp-api-edge.music.apple.com` | Editorial groupings (New, Radio, grouping pages), rooms, multirooms, catalog search, search suggestions, search landing | An edge-cached variant of the same API. Verified: `/v1/editorial/{sf}/groupings` returns 200 on `amp-api` as well. Sona can call either host; edge is what the web uses. |
| `api.music.apple.com` | Public MusicKit API (Sona's current default `BaseURL`) | **It cannot serve Home:** `/v1/me/recommendations?name=listen-now` returns 400 `Platform web is not supported` with `platform=web`, and 400 `A single platform must be supplied` without it. Lyrics also return 400 (`'lyrics' entities require permissions`). Several `views`/`extend` options are also missing here. |
| `play.music.apple.com` | `POST /WebObjects/MZPlay.woa/wa/webPlayback` | The current web app uses this host. `play.itunes.apple.com` (Sona's current one) returns an identical 200 body. |
| `play.itunes.apple.com` | `widevineCert`, `acquireWebPlaybackLicense`, HLS `playlist.m3u8` | Unchanged. Already implemented in Sona. |
| `universal-activity-service.itunes.apple.com/play` | Play activity (`JSPLAY` events) | Already implemented in Sona (`internal/playback/activity`). The web request body embeds the developer token. |
| `amp-api-edge.media.apple.com/v1/engagement/{sf}/upsell` | Marketing and upsell banners | Web-only. Not applicable to Sona. |

### Common query parameters

- `platform=web`: sent on almost every amp-api call. It unlocks web attributes (`editorialVideo`, `canvas` views, personal recommendations).
- `l=en-GB`: localisation, one of the storefront's `supportedLanguageTags` (`/v1/me/storefront`); without it Apple answers in the storefront's `defaultLanguageTag`. Sona sends it on content GETs when the user's language differs from the default and the storefront offers it (`applemusic.ContentLanguage`). Lyrics use `l[lyrics]=en-gb&l[script]=en-Latn` instead, and get no `l=`.
- `format[resources]=map`: a **normalised response**. `data` (or `results.*.data`) holds only `{id,type,href}` references, and full objects live in `resources[type][id]`. Sona's decoders must resolve references through the `resources` map whenever this parameter is sent.
- `art[url]=f` (sometimes `c,f`; per-type `art[music-videos:url]=c`): artwork URL variants. `f` returns the plain template; `c` adds the crop-code template (`…/{w}x{h}{c}.{f}`) that editorial artwork uses `[INFERENCE]`. Artwork `width`/`height` are the source's size (2.4:1 `superHeroWide`, 4:1 `subscriptionHero`/`subscriptionCover` and editorial element art, 16:9-ish video frames). What the CDN returns for a filled `{w}x{h}` box depends on the crop code (checked 2026-11-01): `bb` fits the image inside the box; `sr` and named crops (`SC.DN02`, …) crop to the box (at the source's own aspect ratio the same bytes as `bb`; at another aspect ratio the centre, so a 4:1 `subscriptionHero` at 1060×608 is its middle); `cc` returns a w×w centre square whatever h is (chart playlists list 4320×1080 but are square); `mv` sizes by w alone. No code upscales beyond the source. Sona fills `{c}` with `sr` and requests each box at its own width and height (`internal/artwork`).
- Hero shelf on music.apple.com (Browse/New, checked 2026-11-01): the server-rendered page (`serialized-server-data`) lists it as `hero-list-shelf` with `itemKind: flowcaseLockup`. Each card's art is the item's `editorialArtwork.subscriptionHero` (4320×1080), or the editorial element's own `artwork` for kind `320` cards, with `cropStyle: sr`; the `<picture>` srcset asks for 300×172 to 1060×608 (`…/1060x608sr.webp`), so the card is 1060:608 and the CDN crops the centre of the 4:1 art. The 4:1 art is drawn for that: title and subject sit in the centre, the sides are background. `superHeroWide` (2.4:1) is not used there; on its own it often leaves large dark areas (it is laid out for text drawn over it).
- `omit[resource]=autos` / `omit[resource:artists]=autos`: drops auto-generated relationship stubs.
- `extend=…`: extra attributes, e.g. `editorialArtwork`, `editorialVideo`, `plainEditorialNotes`, `artistBio`, `hero`, `offers`, `seoTitle`, `extendedAssetUrls`, `lyricsExcerpt`, `trackCount`, `followState`, `hasCollaboration`, `artistUrl`. Per-type: `extend[stations]=airTime,supportsAirTimeUpdates`.
- `include=…` / `include[type]=…`: related resources, e.g. `include[songs]=artists,albums`, `include[stations]=radio-show`, `include[library-playlists]=catalog`.
- `relate=…` / `relate[type]=…`: relationship references without full inclusion. `relate=library` adds a `library` relationship to catalog items.
- `views=…`: named sub-collections, e.g. album `more-by-artist`, artist `canvas`, playlist `more-by-curator`.
- `fields[type]=…`: sparse fieldsets. `meta=sorts` lists the available library sort orders.
- `with=…`: server features, e.g. `library`, `social`, `friendsMix`, `serverBubbles`, `naturalLanguage`, `lyricHighlights`, `cityCharts`, `dailyGlobalTopCharts`, `lightProfiles`.
- `timezone=+00:00` (URL-encode the `+` as `%2B`): sent with personal recommendations.

### "Library state" hydration (used on every page)

After any page with catalog items loads, the web app batches two follow-up calls. They decide whether each item shows add/remove, favourited or suggest-less state:

- `GET amp-api /v1/catalog/{sf}?ids[albums]=…&ids[songs]=…&ids[playlists]=…&ids[artists]=…&ids[music-videos]=…&fields[<type>]=inLibrary,personalRating&relate=library&omit[resource]=autos&format[resources]=map&art[url]=f`
  → **200**, MUT: yes for the personal fields. Each item gets `attributes.inLibrary` (bool) and `relationships.library.data[0].id` (its `i.…`/`l.…`/`p.…` library ID). `personalRating` is set when rated.
- `GET amp-api /v1/me/library?ids[library-songs]=…&ids[library-playlists]=…&ids[library-albums]=…&ids[library-artists]=…&fields[<type>]=personalRating&relate=library`
  → **200**. Note: an empty `ids[library-playlist-folders]=` makes it return 400. Skip empty keys.

### App-shell calls (made on every page)

| Purpose | Call | Status | MUT |
|---|---|---|---|
| Subscription / capabilities | `GET /v1/me/account?meta=subscription&challenge[subscriptionCapabilities]=voice,premium` | 200. `accounts` with `attributes{avatarArtwork, restrictions}` and `meta.subscription{active, storefront}`, `meta.challenge` | yes |
| Storefront | `GET /v1/me/storefront` | 200. `storefronts{defaultLanguageTag, explicitContentPolicy, supportedLanguageTags}` | yes |
| Sidebar pins | `GET /v1/me/library/pins?meta=libraryPin&limit=6&include[library-songs]=albums,playlists,artists&include[library-music-videos]=albums,playlists,artists&include[library-artists]=catalog&fields[artists]=artwork` | 200. Library resources, each with `meta.libraryPin{action: play|drillIn|shuffle, positionUUID}` | yes |
| Sidebar playlist tree | `GET /v1/me/library/playlist-folders/p.playlistsroot/children?include=catalog&extend=hasCollaboration&extend[library-playlists]=tags&fields[playlists]=curatorName&offset=0`, then `…/playlist-folders/{folderId}/children` for each folder | 200. A mix of `library-playlists` and `library-playlist-folders`, with `meta.total`. Folder items carry `canEdit` and `canDelete`; playlists carry `tags` (`favorited` marks the Favourite Songs playlist) | yes |
| Own social profile | `GET /v1/me/social/profile?include=social-profile` | 200. `personal-social-profiles{handle, name, isPrivate, isVerified, isOnboarded, collaborationAllowed, …}` linked to a `social-profiles` resource | yes |
| Badges | `GET /v1/social/badging-map` | 200. `results.badgingMap` | yes |
| Upsell | `POST amp-api-edge.media.apple.com/v1/engagement/{sf}/upsell` | 200 | Web-only, skip |

## 2. Top-level navigation

### 2.1 Home (`/{sf}/home`, which replaces "Listen Now"; the bundle also routes `/listen-now`, `/for-you`)

What it shows is an ordered list of personal-recommendation shelves. Their title, kind and display style all come from the server. The observed set, in order:

| Shelf (title from API) | `attributes.kind` | `display.kind` | Content types |
|---|---|---|---|
| Top Picks for You | `music-recommendations` | `MusicNotesHeroShelf` | albums, playlists, stations (each item carries a reason label, e.g. "New Release" or "Made for You") |
| Recently Played | `recently-played` | `MusicCoverShelf` | albums, library-playlists, playlists, stations. `hasSeeAll: true` |
| New Releases for You | `music-recommendations` | `MusicCoverShelf` | albums. `hasSeeAll: true` |
| Genre shelves ("Pop", "R&B", "Country", "Dance", "2010s", …) | `music-recommendations` | `MusicCoverShelf` | albums, playlists |
| Stations for You | `music-recommendations` | `MusicCoverShelf` | stations (artist stations `ra.<artistId>`, personal `ra.u-…`, discovery `ra.q-…`) |
| Concerts | `concerts-for-you` | `MusicConcertsEmptyShelf` | editorial-items (prompt to set a location; Sona shows its own concerts shelf in its place) |
| "More Like …", "More from …", "World of …", "… Fans Like" | `music-recommendations` | `MusicCoverShelf` | albums, playlists |
| Find Your Mood | `music-recommendations` | `MusicCoverShelf` | mood stations (`ra.q-…`) |
| Playlists Made for You | `music-recommendations` | `MusicNotesHeroShelf` | personal mixes (`pl.pm-…`: Favourites/New Music/Chill/Get Up! mixes, Heavy Rotation, Your Essentials) |
| (untitled hero) | `music-recommendations` | `MusicSuperHeroShelf` | one station (the user's personal station) |
| Friends Are Listening To | `music-recommendations` | `MusicCoverShelf` | albums, playlists (needs `with=social`) |
| Favourite Artists | `your-favorites` | `MusicCoverShelf` | artist stations. `hasSeeAll: true` |
| Replay: Your Top Music | `music-recommendations` | `MusicNotesHeroShelf` | editorial-items plus Replay playlists (`pl.rp-…` per year and all-time) |

Actions: play a shelf item directly, open it, use its context menu, and "See All" on shelves with `hasSeeAll`. Home's footer storefront picker is web-only.

API:

| Call | Status | MUT | Response |
|---|---|---|---|
| `GET amp-api /v1/me/recommendations?name=listen-now&platform=web&timezone=%2B00:00&types=activities,albums,apple-curators,artists,concerts,curators,editorial-items,library-albums,library-playlists,music-movies,music-videos,playlists,social-profiles,social-upsells,songs,stations,tv-episodes,tv-shows,uploaded-audios,uploaded-videos&with=friendsMix,library,social&displayFilter[kind]=MusicCircleCoverShelf,MusicConcertsEmptyShelf,MusicCoverGrid,MusicCoverShelf,MusicNotesHeroShelf,MusicSocialCardShelf,MusicSuperHeroShelf&extend=editorialArtwork,editorialVideo,plainEditorialCard,plainEditorialNotes&extend[playlists]=artistNames&extend[stations]=airTime,supportsAirTimeUpdates&include[personal-recommendation]=primary-content&include[albums]=artists&include[library-playlists]=catalog&include[stations]=radio-show&meta[stations]=inflectionPoints&fields[artists]=name,artwork,url&art[url]=f&format[resources]=map&omit[resource]=autos&l=en-GB` | 200 (~1 MB) | yes (403 without) | `data[]` = `personal-recommendation` (each has `attributes{title.stringForDisplay, kind, display.kind, resourceTypes, hasSeeAll, nextUpdateDate, version}` and `relationships.contents` / `primary-content`). `resources` hold `albums`, `artists`, `playlists`, `stations`, `editorial-items`, `library-playlists`. `meta.cachePolicy.updateDate` gives the refresh time. |
| `POST amp-api /v1/me/recommendations/{recoId}?name=listen-now&action=refresh&…same types/extend…&with=library` (no body) | 200 | yes | One refreshed `personal-recommendation`. The web calls it for the Recently Played shelf after load. |
| `GET amp-api /v1/me/recommendations/{recoId}/view/see-all?name=listen-now&action=refresh&extend=artistUrl&include[personal-recommendation]=contents&include[library-playlists]=catalog&limit[contents]=100&with=library&timezone=…` `[bundle]` | 200 | yes | The same `personal-recommendation`, with up to 100 contents. This is the "See All" grid. |
| `GET api /v1/me/recommendations?limit=10` (public API) | 200 | yes | The legacy "For You" set: `personal-recommendation` with `kind`, `title` and `contents` but no `display` and no shelf ordering. **Not equivalent to Home.** |
| `GET api /v1/me/history/heavy-rotation?limit=10` | 200 | yes | library-playlists, library-albums, stations, playlists |
| `GET api /v1/me/recent/played?limit=10` | 200 | yes | albums, playlists, stations (container-level recently played) |

### 2.2 New (`/{sf}/new`; also `/{sf}/browse`)

What it shows: one editorial grouping. Its `tabs[0].children` are `editorial-elements` sections, each with `name`, `editorialElementKind` and a `contents` relationship. Observed sections: hero carousel (kind `316`, nested editorial-elements linking to albums, playlists, music videos and stations), Best New Songs (`327`, songs), New This Week / Recent Releases / Only on Apple Music / Latest DJ Mixes / Coming Soon (`326`, albums), Updated Playlists / New in Music / Now on Tour (playlists), "New for October" (`385`, brick of editorial-elements), Trending Songs, Everyone's Listening To, Daily Top 100 and City Charts (playlists, which link to Charts), Listen to Live Radio Now and New Radio Episodes (stations), Watch Interviews and New Music Videos (music-videos), and More to Explore (`391`, links to groupings and rooms).

Actions: open or play items, "See All" (opens the section's `room`), and context menus.

| Call | Status | MUT | Response types |
|---|---|---|---|
| `GET amp-api-edge /v1/editorial/{sf}/groupings?name=music&tabs=subscriber&platform=web&art[url]=c,f&extend=artistUrl,editorialArtwork,plainEditorialNotes&extend[station-events]=editorialVideo&fields[albums]=artistName,artistUrl,artwork,contentRating,editorialArtwork,plainEditorialNotes,name,playParams,releaseDate,url,trackCount&fields[artists]=name,url,artwork,editorialArtwork,genreNames,plainEditorialNotes&include[albums]=artists&include[music-videos]=artists&include[songs]=artists&include[stations]=events,radio-show&relate[songs]=albums&omit[resource:artists]=autos&format[resources]=map` | 200 (~1.4 MB) | **no** | `groupings` → `editorial-elements` → `albums`, `playlists`, `songs`, `music-videos`, `stations`, `station-events`, `apple-curators`, `rooms`, `uploaded-videos`, `artists` |

### 2.3 Radio (`/{sf}/radio`)

What it shows: the same grouping mechanism with `name=radio`. Observed sections: hero (`316`), "On Air Now" live stations with schedules (kind `488`: Apple Music 1, Hits, Country, Club, Chill and others, each `isLive:true`), Latest Radio Episodes, Live from London, Artists Take Over, Listen to Interviews (stations, i.e. radio episodes `ra.<id>` with `airTime`), Watch Interviews (music-videos, uploaded-videos), Apple Music Sessions / Best Club DJ Mixes (albums), Shows Hosted by Artists / Our Radio Hosts / All Shows (`apple-curators` of `kind: Show`), Local Broadcasters (third-party live stations such as Capital and KISS), Top Stations and Stations by Genre (algorithmic genre stations), and More to Explore.

The live-station schedule page (`/{sf}/radio/{stationId}`) lists `station-events` (`title, description, startTime, endTime, artwork, heroArtwork, editorialVideo`). A programme's `artwork` and `heroArtwork` are 4320×1080 key art (`{w}x{h}bb.{f}` with `art[url]=f`, `{w}x{h}{c}.{f}` with `c`), drawn so the centre square is the programme's tile: the web player shows each programme as a square lockup of `artwork` with the `cc` crop (`296x296cc.webp` to `632x632cc.webp`, checked 2026-11-01). Sona's schedule asks for the same square (`cc`); the `bb` template fitted into a square box returns a strip a quarter of the box's height.

| Call | Status | MUT | Response types |
|---|---|---|---|
| `GET amp-api-edge /v1/editorial/{sf}/groupings?name=radio&tabs=subscriber&…` (same params as New) | 200 | no | `groupings`, `editorial-elements`, `stations`, `station-events`, `apple-curators`, `rooms`, `albums`, `music-videos`, `uploaded-videos`, `artists` |
| `GET amp-api /v1/me/recent/radio-stations?limit=30&include[stations]=radio-show&omit[resource]=autos` `[bundle]` | 200 | yes | stations (`isLive`, `radioUrl`, `supportedDrms`, `requiresSubscription`, `stationProviderName`) |
| `GET amp-api /v1/catalog/{sf}/stations?filter[featured]=apple-music-live-radio` | 200 | no | 12 live stations |
| Schedule: `GET amp-api /v1/catalog/{sf}/stations/{ra.id}?include=curator&include[stations]=events&art[url]=c,f` | 200 | no | `stations` → `events` (`station-events`), `apple-curators` |

### 2.4 Search (`/{sf}/search`, `?term=…`, `&searchIn=library`)

**Empty state ("Browse Categories")** is a grid of category tiles. Each tile is an `apple-curators` resource of `kind: Genre` (Alternative, Hits, Pop, Charts, Fitness, Sleep, decades, moods and so on) linking to a curator page. Some tiles are multirooms or curators for campaigns. Recent searches show above the grid.

- Recent searches are **client-side only**: the web keeps them in browser storage under `RecentlySearchedItems`, and it also merges recent library searches. They can be cleared. No API is involved, so Sona keeps them in SQLite.
- `GET amp-api-edge /v1/recommendations/{sf}?name=search-landing&types=activities,apple-curators,editorial-items&extend=editorialArtwork&art[url]=c,f&format[resources]=map&omit[resource]=autos` → **200**, MUT: no. `personal-recommendation` blocks (`display.kind: MusicNamedBrickShelf`) containing `apple-curators` and `editorial-items`.

**Search hints (as you type)** come in two kinds: term completions and "top results" lockups (artist, playlist, album, song).

- `GET amp-api-edge /v1/catalog/{sf}/search/suggestions?term=…&kinds=terms,topResults&limit[results:terms]=5&limit[results:topResults]=10&types=activities,albums,artists,editorial-items,music-movies,music-videos,playlists,record-labels,songs,stations,tv-episodes&with=naturalLanguage&fields[albums]=artistName,artwork,contentRating,name,playParams,url&fields[artists]=url,name,artwork&art[url]=f&format[resources]=map&omit[resource]=autos` → **200**, MUT: no. `results.suggestions[]` has `{kind: "terms", displayTerm, searchTerm}` or `{kind: "topResults", content: {id,type}}`. The web fires this per keystroke; debounce it.

**Results, Apple Music scope.** Sections come in `meta.results.order`: top, artist, album, song, playlist, radio_show, radio_episode, station, music_video, video_extra. `rawOrder` also lists category, curator and record_label. Social profiles and user playlists come from a parallel social search ("Profiles" section). The old public-API call (`GET api /v1/catalog/{sf}/search?types=songs,…`) returns no top-results group, no radio episodes and no ordering; Sona no longer uses it.

| Call | Status | MUT | Response |
|---|---|---|---|
| `GET amp-api-edge /v1/catalog/{sf}/search?term=…&limit=21&types=activities,albums,apple-curators,artists,curators,editorial-items,music-movies,music-videos,playlists,record-labels,songs,stations,tv-episodes,uploaded-videos&with=lyricHighlights,lyrics,naturalLanguage,serverBubbles,subtitles&extend=artistUrl&include[albums]=artists&include[songs]=artists&include[music-videos]=artists&include[stations]=radio-show&relate[albums]=artists&relate[songs]=albums&fields[albums]=…&fields[artists]=url,name,artwork&art[url]=f&art[music-videos:url]=c&format[resources]=map&omit[resource]=autos` | 200 | no | `results.{top, artist, album, song, playlist, radio_episode, station, music_video, video_extra}.data` → albums, artists, songs, playlists, stations, music-videos, uploaded-videos, apple-curators, music-movies. Per-group pagination uses `results.<group>.next`. |
| `GET amp-api /v1/social/{sf}/search?term=…&types=playlists,social-profiles&limit=21&with=lyrics,serverBubbles&art[social-profiles:url]=c` | 200 | **yes** (403 without) | `results.{top, playlist, profile}` → `social-profiles`, user `playlists` |
| "See all" page of one group: the same call plus `groups=<group>&offset=N` (the group's `next` href carries both) | 200 | no | `results.<group>` only. **Needs `limit` and `with=serverBubbles`**: without them the page comes back with empty `results`. `next` is null on a group's last page; `top`, `station` and `radio_show` usually have none. |
| Social group page: `GET amp-api /v1/social/{sf}/search?…&groups=profile|playlist&offset=N` | 200 | yes | `results.{profile|playlist}` |
| Profile page (`/profile/{handle}`): `GET amp-api /v1/social/{sf}/social-profiles?ids={sp.…}&include=shared-playlists&extend=followState&art[url]=c,f&format[resources]=map` (by handle the bundle sends `filter[handle]=…` instead of `ids`; not verified) | 200 | yes | `social-profiles{name, handle, artwork, isPrivate, isVerified, followState, url}` → `shared-playlists` (public `pl.u-…` playlists). Including `listening-to`, `followers` or `followees` fails with 400 "Invalid Request Signature" (the web signs those requests); Sona omits them. |

Sona calls the catalog search, suggestions and landing on `amp-api` (identical 200 responses to `amp-api-edge`, verified).

**Results, Library scope** (a toggle; the route gains `searchIn=library`):

- `GET amp-api /v1/me/library/search?term=…&types=library-albums,library-artists,library-playlists,library-songs&limit=21&include[library-songs]=catalog&include[library-artists]=catalog&format[resources]=map&art[url]=f` → **200**, MUT: yes. `results.{library-playlists, library-artists, library-songs, library-albums}` with `meta.results.order`. Sona already answers library search locally from SQLite FTS. Keep it that way.

## 3. Library

Sidebar sections: Recently Added, Artists, Albums, Songs, Music Videos, Made for You, then a Playlists group (All Playlists, Favourite Songs, then the folder tree and playlists), plus Pins at the top (up to 6 pinned items with a play / drill-in / shuffle action). **There is no Downloaded section on the web.** Downloads don't apply to web or Sona (streaming only, no offline DRM).

| Page | Route | Call (all `amp-api`, MUT: yes, 403 without) | Status / shape |
|---|---|---|---|
| Recently Added | `/{sf}/library/recently-added` | `GET /v1/me/library/recently-added?limit=25&offset=N&includeOnly=catalog,artists&include[library-albums]=artists&include[library-artists]=catalog&fields[albums]=…&format[resources]=map&omit[resource]=autos` | 200. `next` link; `library-albums` (`dateAdded`) and `library-playlists`, each with a `catalog` relationship. Pages of 25. |
| Artists | `/{sf}/library/artists`, detail `/library/artist/{r.id}` | `GET /v1/me/library/artists?limit=100&offset=N&meta=sorts&include=catalog&sort=name`. Detail: `GET /v1/me/library/artists/{id}?include[library-artists]=albums,catalog&include[library-albums]=catalog&fields[library-artists]=classicalUrl,name,url` | 200. `meta{total, sorts:[name], currentSort}`; `library-artists{name}` with catalog `artists` (artwork comes from the catalog). |
| Albums | `/{sf}/library/albums`, detail `/library/albums/{l.id}` | `GET /v1/me/library/albums?limit=100&offset=N&meta=sorts&sort=name&includeOnly=catalog,artists&include[library-albums]=artists&include[albums]=artists`. Detail: `GET /v1/me/library/albums/{id}?includeOnly=catalog,artists,tracks&include[library-albums]=artists,tracks&include[albums]=artists,tracks&include[music-videos]=catalog,artists,tracks&extend[albums]=editorialArtwork,editorialVideo,extendedAssetUrls` | 200. `meta.sorts: name, dateAdded`. `library-albums{artistName, artwork, dateAdded, genreNames, name, playParams, releaseDate, trackCount}`. No `isSingle`/`isCompilation`: `include=catalog` (with `fields[albums]=isSingle,releaseDate,isPrerelease` to keep it small, as Sona's sync does, on the web host: §3.1) brings the catalog album's flag; `trackCount` counts the tracks in the library, not the release's. Names of singles and EPs end in " - Single" / " - EP" (English storefronts), and the catalog flags singles but not EPs. |
| Songs | `/{sf}/library/songs` | `GET /v1/me/library/songs?limit=100&offset=N&meta=sorts&sort=name&include[library-songs]=catalog` | 200. `meta.sorts: name, dateAdded`. `library-songs{albumName, artistName, artwork, discNumber, durationInMillis, genreNames, hasLyrics, name, playParams, releaseDate, trackNumber}`. |
| Music Videos | `/{sf}/library/music-videos` | `GET /v1/me/library/music-videos?limit=100&offset=0&meta=sorts&include[library-music-videos]=catalog` | 200. Empty for this user: `meta.total: 0`, empty-state section. |
| Made for You | `/{sf}/library/made-for-you` | `GET /v1/me/library/playlists?filter[featured]=made-for-you&sort=type&include[library-playlists]=catalog&fields[library-playlists]=artwork,dateAdded,name,playParams&fields[playlists]=lastModifiedDate` | 200. The library copies of the personal mixes; `meta.filters.featured["made-for-you"]`. Sona does not call it: the sidebar's Made for you section picks the synced library playlists whose catalog ID starts `pl.pm-` (the personal-mix ID kind below). |
| All Playlists | `/{sf}/library/all-playlists/` | Folder tree (see the shell table), plus `GET /v1/me/library/playlists/{id}/tracks?include[songs]=artists,composers&fields[library-songs]=id,artwork,url&fields[artists]=name,url` per visible playlist (for the artwork mosaic) | 200 |
| Playlist folder | `/library/playlist-folder/{p.id}` | `GET /v1/me/library/playlist-folders/{id}/children?offset=0&extend=hasCollaboration&extend[library-playlists]=tags` | 200 |
| Favourite Songs | `/library/playlist/{favId}` (tagged `favorited`; the bundle also has `/library-mix/favorite-songs/{id}`) | Same as library playlist detail below. `GET /v1/me/library/playlists?filter[library-mix]=favorite-songs` returned 200 with an empty `data`. | 200 |
| Library playlist | `/{sf}/library/playlist/{p.id}` | `GET /v1/me/library/playlists/{id}?include=catalog,artists,tracks&include[library-playlists]=catalog,tracks,playlists&include[playlists]=curator&include[songs]=artists&extend=hasCollaboration,isCollaborativeHost&extend[library-playlists]=tags&extend[playlists]=editorialArtwork,editorialVideo,extendedAssetUrls&fields[songs]=artistUrl,artwork,contentRating,durationInMillis,url&fields[music-videos]=artistUrl,artwork,durationInMillis,url&art[library-music-videos:url]=c,f&relate=catalog` | 200. `library-playlists{canDelete, canEdit, dateAdded, description, hasCatalog, hasCollaboration, isCollaborativeHost, isPublic, lastModifiedDate, name, playParams, tags}` with `tracks` (`library-songs` and their catalog `songs`). |
| Suggested songs (shown below a user playlist) | same page | `POST /v1/me/recommendations/suggested?contexts=playlist-suggested-songs&types=songs&limit=20&meta=source&include[songs]=artists&fields[songs]=artwork,name,url,artistName,albumName,previews` with body `{"targetContent":{"id":"p.…","type":"library-playlists"}}` | 200. `results.suggested` → songs |
| "Add to Playlist" target list | context menu | `GET /v1/me/library/playlists?filter[featured]=suggested` | 200. Recently used editable playlists (`meta.filters.featured.suggested`) |

Library list paging: `limit` is at most 100 and `meta.total` is present. This matches Sona's `FetchPages`. Library playlists can mix catalog and uploaded or non-catalog songs; resolve `catalog` relationships when present.

### 3.1 A pre-added (unreleased) album in the library

Checked live on 2026-11-10 (`SONA_PRIVATE=1`, read-only, through `applemusic.Client`) against a library of 654 albums and 1,076 songs that held one pre-added album, still 10 days from its release. The shapes below are what Apple answered, with no names or IDs.

- **The library album** (`GET /v1/me/library/albums`, `library-albums`): `artistName`, `artwork`, `dateAdded`, `genreNames`, `name`, `playParams {id, isLibrary: true, kind: "album"}` and `trackCount` (all 34 of the release's tracks, each of which is in the library). **There is no `releaseDate`**: the key is absent (two albums of the library lack it; the other is a plain album with no catalog album). Nothing marks it as unreleased.
- **Its catalog album** (`include=catalog`, `relationships.catalog.data[0]`): `releaseDate: "2026-11-20"`, the day it comes out, with `isComplete: false`. `isPrerelease: true` is returned only by the web host, `amp-api.music.apple.com`, with `platform=web`: `api.music.apple.com` leaves it out under `fields[albums]=isPrerelease` and under no `fields`, and rejects `platform=web` on the single album route with a 400. With `fields[albums]=isSingle,releaseDate,isPrerelease` the amp-api answer is just those three attributes. The catalog album page (`GET /v1/catalog/{sf}/albums/{id}`, default fields) carries `isPrerelease: true` too, which is what the album page reads.
- **Its songs** (`GET /v1/me/library/songs`, and the album's `tracks`): all 34 are listed with an `i.…` library ID. The ones already out (8, released 5 to 8 weeks before, plus one that has `playParams` but no date) carry `releaseDate`, `playParams {catalogId, id, isLibrary, kind: "song", reporting, reportingId}`, `artwork` and `contentRating`. The rest (25) are named "Track N" and have **no `playParams`, no `releaseDate` and no `artwork`**: only `albumName`, `artistName`, `discNumber`, `durationInMillis`, `genreNames`, `hasLyrics`, `name`, `trackNumber`. Their catalog songs mostly lack them too. 4 more songs of the library, on albums that are out, have no `playParams` either (not on this album); Sona leaves those as they are.
- **Refreshing** one: `GET /v1/me/library/albums?ids=l.…` (or `…/songs?ids=i.…`, comma separated, at most 100) with the walk's other query. Apple answers 400 ("Limit may not be supplied on this request") when `limit` or `offset` goes with `ids`. An ID no longer in the library is omitted.

What Sona does with it: the albums walk asks the web host for `isSingle,releaseDate,isPrerelease` of the included catalog album and stores `albums.is_prerelease` and `albums.catalog_release_date` (the library's `release_date` stays as it is); a library song stores `tracks.has_play_params`. An album is upcoming when the catalog's date is ahead of now (it counts when the library has none or the library's is passed), and by Apple's flag only without a full date (a year alone, or none). A song with no `playParams` or no date of its own goes by its album. The newest-first walks stop at the first row they have, so each incremental sync asks again for the albums still ahead (flagged, or dated ahead), and for those albums' songs that have no `playParams` yet; once Apple dates the album and drops the flag the row clears. Rows stored before the fields existed are brought right by the full run that migration 00012 forces on the albums and songs domains.

## 4. Detail pages

### 4.1 Album (`/{sf}/album/{slug}/{id}`; `?i={songId}` highlights a track)

Data: title, artist(s) (multiple links), genre, release year/date, `editorialNotes` (`short`/`standard`), `editorialArtwork`, `editorialVideo` (motion artwork), the track list with `discNumber` grouping and per-track `meta.popularity`, `contentRating` (explicit), `audioTraits` (`lossless`, `hi-res-lossless`, `lossy-stereo`, `atmos`, `spatial`), `isAppleDigitalMaster`, `isMasteredForItunes`, `copyright`, `recordLabel` (plus `record-labels` relationship), `upc`, `isSingle`, `isCompilation`, `isComplete`, `isPrerelease`, `trackCount`, total duration (computed). Sections come from `views` in `meta.views.order`: other-versions, related-videos (music videos), video-extras, audio-extras, more-by-artist, appears-on ("Featured On" playlists), and you-might-also-like.

Actions: play, shuffle, add album to library / delete from library, favourite album, per-track context menu (see §5.6), Copy Link, Copy Embed Code.

| Call | Status | MUT | Response |
|---|---|---|---|
| `GET amp-api /v1/catalog/{sf}/albums/{id}?views=appears-on,audio-extras,more-by-artist,other-versions,related-videos,video-extras,you-might-also-like&extend=editorialArtwork,editorialVideo,extendedAssetUrls,offers,seoDescription,seoTitle&include=record-labels,artists&include[songs]=artists,composers,albums&include[music-videos]=artists&include[playlists]=curator&fields[artists]=name,url&fields[curators]=name&fields[record-labels]=name,url&meta[albums:tracks]=popularity&art[url]=f&format[resources]=map` | 200 | no | `albums` (rel: artists, record-labels, tracks; 7 views) → `songs`, `music-videos`, `playlists`, `apple-curators`, `artists` |
| Public-API equivalent `GET api /v1/catalog/{sf}/albums/{id}?views=…` | 200 | no | Only `appears-on`, `other-versions` and `related-videos` come back (**no** `more-by-artist`, `you-might-also-like` or `audio-extras`). No `audioTraits` or `editorialVideo`. |

Fields the album page reads beyond the obvious ones (observed in gb, November 2026):

- `artwork.bgColor` and `artwork.textColor1`–`textColor4` (`rrggbb`): the colour the cover sits on and the text colours Apple sets over it (1 and 2 primary and secondary, 3 and 4 the same blended towards the background). They are not contrast-safe on their own: Metallica's black album gives `textColor1` = `textColor2` = `dfebe5`; BRAT's lime gives near-black `050800`/`070a00`; Mellon Collie's navy gives gold `fdc14c`. Every resource with `artwork` carries them, songs included.
- `genreNames` lists the main genre, the root genre ("Music", localised: "Musique", "ミュージック"), then subgenres, with repeats. `include=genres` (`fields[genres]=name,parentId`) returns the top-level genres and the root, the one without `parentId`, which tells the root's name in any language.
- Album tracks carry `composerName` (songwriters joined with ", " and " & ": "Taylor Swift, Jack Antonoff & Zoë Kravitz"), and classical tracks `workName`, `movementName`, `movementNumber`, `movementCount` and `attribution` (the composer): "Goldberg Variations, BWV 988 (1955 Recording)" / "Aria". Each track's `releaseDate` is its own first release, so a reissue or compilation dates its recordings earlier than the album (the Goldberg set of 2013: 1955 to 2000).
- `views=audio-extras` ("More to Hear") is radio episodes about the album (`stations` with `kind: streaming`, e.g. Apple Music 1's "100 Best Albums" episode, an artist interview); `video-extras` ("More to See") was empty on every album sampled. `include[songs]=composers` relates songs to composer `artists`; album-level credits (performers, producers) exist only per song (`include=credits`, §4.3).

Sona today: `AlbumPage` asks for six of those views (all but `video-extras`), `include=artists,genres` and `extend=editorialVideo`; the album DTO carries the artwork colours, the genres without the root, each track's composer, work and movement, and the episodes as "More to hear".

#### Motion artwork (`editorialVideo`, checked 2026-11-06 on us albums)

`extend=editorialVideo` returns flavours keyed by name, each `{video: <HLS master>, previewFrame: {url, width, height}}`. On albums: `motionDetailSquare` and `motionSquareVideo1x1` (square; both carried the same ladder), `motionDetailTall` and `motionTallVideo3x4` (3:4). An artist's is `motionArtistSquare1x1` (and wide flavours). Apple's Album Motion Guidelines (help.apple.com/itc/albummotionguide) name the deliverables behind them: "Album Page Motion 3x4" (2048×2732, iPhone and Android, with a safe area because the phone's text and gradients overlay its top and bottom) and "Album Page Motion 1x1" (3840×3840, Mac, iPad and smart TVs, no UI over it); the first frame matches the static cover and the animation loops with no visible join. Each master lists SDR H.264 (`avc1.64001f`/`avc1.640020`) and HEVC (`hvc1.2.20000000…`) renditions: the square H.264 ladder is 360, 408, 456, 486 (three bit rates), 768, 960 (two) and 1080 (four) px wide, HEVC adds 1920 and 2160 (the 3840 master is not offered); the 3:4 one runs 310×414 to 2048×2732. 30 fps, one fragmented MP4 read by byte ranges; the first frame is a keyframe at 0 and a loop was 22.6 s of 679 frames of 33.3 ms.

Sona plays the square flavours only (a desktop is the Mac and iPad class, and a 3:4 picture cropped into a square cover would lose its safe area), the first present of `motionDetailSquare`, `motionSquareVideo1x1`, `motionArtistSquare1x1`; `previewFrame` is not used (the still is the album's own cover). The cache chooses the smallest H.264 rendition at least as wide as the cover draws, 360, 486, 768 or 960 px, at the highest bit rate up to 4 Mbit/s.

### 4.2 Artist (`/{sf}/artist/{slug}/{id}`)

Data: name, `artwork`, `editorialArtwork` / `hero` (banner), `editorialVideo`, `artistBio` (with `hasBiographyContent`), `genreNames`, `origin`, `bornOrFormed`, and `default-playable-content` (the top song played by the header Play button). The web builds the page from a server **canvas**: `views.canvas` is an ordered list of `canvas-shelves-header`, `canvas-shelves-group` and `canvas-shelves-collection` items. Each collection has a `title`, `resourceTypes` and a `see-all` relationship. Observed order: Latest Release, Top Songs, Essential Albums, Albums, Singles & EPs, Music Videos, All Upcoming Concerts, Artist Playlists, Radio Shows, Live Albums, Compilations, Appears On, More To Hear (stations), More To See (uploaded-videos, music-movies), Similar Artists.

Actions: play (default playable content), Create Station, Favourite artist, Suggest Less, Copy Link, and See All per shelf.

| Call | Status | MUT | Response |
|---|---|---|---|
| `GET amp-api /v1/catalog/{sf}/artists/{id}?views=canvas&include=default-playable-content&include[canvas-shelves]=contents,children&include[concerts]=artists,venues&include[songs]=albums&relate[canvas-shelves-collection]=see-all&filter[canvas-shelves-collection:contents{type}]=albums,apple-curators,artists,concerts,music-movies,music-videos,playlists,songs,stations,uploaded-videos,venues&limit[canvas-shelves-collection:contents]=12&limit[canvas-shelves-collection:contents{displayStyle}]=artist-featured-track-lockup:24&extend=artistBio,editorialArtwork,editorialVideo,extendedAssetUrls,hasBiographyContent,hero,keyColor,plainEditorialNotes,seoDescription,seoTitle&extend[canvas-shelves-header]=badge&extend[concerts]=tickets&art[url]=c,f&format[resources]=map&omit[resource]=autos` | 200 | no | `artists` → `canvas-shelves-header`, `canvas-shelves-group`, `canvas-shelves-collection`, `albums`, `songs`, `music-videos`, `playlists`, `stations`, `artists`, `uploaded-videos`, `music-movies`, `concerts` |
| Classic views, simpler to consume: `GET amp-api /v1/catalog/{sf}/artists/{id}?views=latest-release,top-songs,full-albums,singles,compilation-albums,live-albums,appears-on-albums,featured-albums,playlists,music-videos,top-music-videos,similar-artists,featured-playlists,radio-shows,more-to-hear,more-to-see&extend=artistBio,bornOrFormed,origin,hero,editorialArtwork,plainEditorialNotes&format[resources]=map&platform=web` | 200 | no | All 16 views are present, with `meta.views.order`. **Recommended for Sona** over parsing the canvas. |
| See All for a view: `GET amp-api /v1/catalog/{sf}/artists/{id}/view/{viewName}?limit=20&with=attributes&include[songs]=artists,composers&include[albums]=artists,composers&include[music-videos]=artists,composers` `[bundle]` | 200 | no | `next`, `data` (albums, songs, …), `attributes` (view title) |
| Canvas see-all collection: `GET amp-api /v1/editorial/{sf}/canvas-shelves-collection/{collectionId}` | 200 | no | `canvas-shelves-collection{title, display, resourceTypes}` |

Sona today: `Artist` asks `api.music.apple.com` for `views=top-songs,full-albums` only.

### 4.3 Playlist (`/{sf}/playlist/{slug}/{pl.id}`)

Playlist ID kinds: `pl.<hex>` editorial, `pl.u-…` user-shared, `pl.pm-…` personal mix (Made for You), `pl.rp-…` Replay.

Data: `name`, `curatorName` + `curator` (an apple-curator, a curator, or the owner's social profile), `description` / `editorialNotes`, `editorialArtwork`, `editorialVideo`, `lastModifiedDate` ("updated"), `playlistType` (`editorial`, `user-shared`, `personal-mix`, `replay`, …), `isChart`, `supportsSing`, `trackCount`, `audioTraits`, `hasCollaboration`, and tracks (songs and music videos, up to 300 per page). Views: `featured-artists`, `more-by-curator`. Social views: `collaborators`, `contributors`, `subscribers` (with `followState`).

Actions: play, shuffle, add to library, favourite, Suggest Less, Play Next / Play Last, Add to Playlist, Copy Link, Copy Embed Code, and per-track menus.

| Call | Status | MUT | Response |
|---|---|---|---|
| `GET amp-api /v1/catalog/{sf}/playlists/{id}?include=tracks,curator&include[songs]=artists&include[music-videos]=artists&views=featured-artists,more-by-curator&limit[tracks]=300&limit[view.featured-artists]=15&limit[view.more-by-curator]=15&extend=editorialArtwork,editorialVideo,offers,seoDescription,seoTitle,trackCount&fields[songs]=name,artistName,curatorName,composerName,artwork,playParams,contentRating,albumName,url,durationInMillis,audioTraits,extendedAssetUrls&fields[albums]=name,artwork,playParams,url&fields[artists]=name,artwork,url&fields[curators]=name,url&fields[apple-curators]=name,url&art[url]=f&format[resources]=map&omit[resource]=autos` | 200 | no | `playlists` (rel: curator, tracks) → `songs`, `artists`, `apple-curators` |
| Parallel social/collab call: `GET amp-api /v1/catalog/{sf}/playlists/{id}?fields[playlists]=tracks&relate[playlists]=tracks&associate[playlists:tracks]=collaborators&meta[playlists:tracks]=contributors&views=collaborators,contributors,subscribers&limit[collaborators]=100&limit[tracks]=300&limit[view.contributors]=15&extend=followState&fields[social-profiles]=name,artwork,handle&with=lightProfiles` | 200 | no | `playlists` (views: collaborators, contributors, subscribers) |
| Sona's collaboration call (`Client.PlaylistCollaboration`, checked 2026-11-02 on a collaborative `pl.u-…`): the call above with `include=curator`, `fields[playlists]=tracks,curator,hasCollaboration`, `extend=hasCollaboration`, `views=collaborators` and `fields[songs]=playParams` (the page has the tracks already) | 200 | no | `attributes.hasCollaboration`; `relationships.curator` is the owner's `social-profiles{name, handle, url, artwork}`; `views.collaborators.data` lists everyone who can add songs, owner first, with `{name, handle, artwork}` (`artwork` absent when the person has none; its template has `{c}`); each track carries `meta.associations.collaborators.data[0]`, the profile that added it (empty for songs added before the playlist became collaborative; people who left are still named here). Only the first 300 tracks are attributed: `limit[tracks]` stops at 300 (400 above), and `…/tracks?offset=N` ignores `associate`. A non-collaborative user playlist answers `hasCollaboration: false` and an empty view. |
| Next pages: `GET amp-api /v1/catalog/{sf}/playlists/{id}/tracks?offset=N&include[songs]=artists` | 200 (404 `No related resources` past the end) | no | `next`, songs |

### 4.4 Song (`/{sf}/song/{slug}/{id}`)

Data: song attributes (see the badges in §5.9), the album it is on, artists, `lyricsExcerpt` (a few lines and "View Full Lyrics"), credits grouped into role categories ("Performing Artists", "Composition & Lyrics", "Production & Engineering") using `credit-artists{name, roleNames, artwork}` (no relationship to an `artists` resource: the id is `<category>-ami:identity:<hash>`; `artwork` is a square picture where Apple has one, otherwise an initials image generated under `/image/thumb/gen/{w}x{h}AM.SCM01.jpg?name=<base64>&signature=…`, or absent; checked 2026-11-03 on songs of Radiohead, Kenshi Yonezu, Fairuz, Yo-Yo Ma and BTS), and "More By …" albums. Actions: play, Create Station, Favourite / Undo Favourite, Show Album, Pin Song (when in library), Delete from Library, Add to Playlist, Play Next / Last, Copy Link / Embed.

| Call | Status | MUT | Response |
|---|---|---|---|
| `GET amp-api /v1/catalog/{sf}/songs/{id}?include=albums,artists,credits,lyrics,music-videos&extend=lyricsExcerpt,offers&fields[albums]=artistName,artistUrl,artwork,name,url&fields[artists]=name,url&art[url]=f&format[resources]=map` | 200 | no | `songs` (rel: albums, artists, credits, lyrics, music-videos) → `role-categories`, `credit-artists`, `lyrics`, `albums` |
| `GET amp-api /v1/catalog/{sf}/songs/{id}/credits` | 200 | no | `role-categories{kind, title}` → `credit-artists` |
| `GET amp-api /v1/catalog/{sf}/songs/{id}/albums?limit=1` (used by "Show Album" and to resolve a song route) `[bundle]` | 200 | no | albums |

### 4.5 Music video (`/{sf}/music-video/{slug}/{id}`)

Data: `name`, `artistName`, `contentRating`, `durationInMillis`, `editorialNotes`, `genreNames`, `has4K`, `hasHDR`, `videoTraits`, `previews`, `releaseDate`. Views: `more-by-artist`, `more-in-genre`. Actions: play (video), add to library, favourite, playlist actions, link/embed.

- `GET amp-api /v1/catalog/{sf}/music-videos/{id}?views=more-by-artist,more-in-genre&include[music-videos]=artists&fields[artists]=name,url&art[url]=f&format[resources]=map&omit[resource]=autos` → **200**, MUT: no. `music-videos`, `artists`.
- Interviews and "posts" are `uploaded-videos`: `/{sf}/post/{id}` → `GET amp-api /v1/catalog/{sf}/contents?ids={id}&include=artists,curator` → **200**, MUT: no. `uploaded-videos{name, artistName, assetTokens, durationInMilliseconds, postUrl, uploadDate, contentRatingsBySystem}` + `apple-curators`.
- Music movies (`/{sf}/music-movie/{slug}/{id}`) → `GET amp-api /v1/catalog/{sf}/music-movies/{id}?views=cross-merchandise,music-artists,playlists&include=grouping,playlists&extend=editorialArtwork` → **200**, MUT: no.

Playback of video (music videos, uploaded videos and movies) uses HLS video assets. Music video playback was spiked on Linux on 2026-11-01 (gb storefront, Widevine CDM 4.10.3112.0, the downloaded desktop CDM, i.e. software L3; two videos: a 1080p H.264-only one and a 4K one with H.264 and HEVC). Sona plays music videos in SD through the CDM's own video decoder (findings below; the design is in `docs/architecture.md` §4.2). Uploaded videos and movies are not played. The metadata pages are cheap.

**Playback info.** `POST webPlayback {"salableAdamId": "<music video id>"}` → **200**, MUT: yes. `songList[0]` has `assets: []` and instead `hls-playlist-url` (a master playlist on `play.itunes.apple.com/WebObjects/MZPlay.woa/hls/playlist.m3u8?cc=…&a=<id>&dsid=…&id=…&aec=HD`; it names the account, so it must never be logged), plus `hls-key-server-url` (`acquireWebPlaybackLicense`), `widevine-cert-url`, `hls-key-cert-url` (FairPlay), `songId`, `is-itunes-stream: true`. A plain `GET` of the master needs no credentials.

**Master playlist.** HLS v7, `#EXT-X-SESSION-DATA` (`com.apple.hls.format` `HD` or `HD,4K`, title, poster, genre, release date). Audio renditions are separate `#EXT-X-MEDIA:TYPE=AUDIO` groups (`audio-HE2-stereo-32` `mp4a.40.29`, `audio-HE-stereo-64` `mp4a.40.5`, `audio-stereo-64`, `audio-stereo-128`, `audio-stereo-256` `mp4a.40.2`); video variants are `#EXT-X-STREAM-INF` with `RESOLUTION`, `CODECS` (`avc1.64001f`/`avc1.640028`, and `hvc1.2.20000000.L123.B0`/`H150` on newer videos), `FRAME-RATE` (25), `VIDEO-RANGE=SDR`, `HDCP-LEVEL` and `ALLOWED-CPC` (`…,urn:uuid:edef8ba9-79d6-4ace-a3c8-27dcd51d21ed:WIDEVINE_SOFTWARE` or `…:WIDEVINE_HARDWARE`), plus `#EXT-X-I-FRAME-STREAM-INF` trick-play variants. Neither video had subtitle renditions (`CLOSED-CAPTIONS=NONE`, no `TYPE=SUBTITLES`).

| Variants | `HDCP-LEVEL` | `ALLOWED-CPC` (Widevine) | Licence for this CDM |
|---|---|---|---|
| Up to 864×486 (16:9) or 864×392 (wide), ≤2.6 Mbps, H.264 and HEVC | `NONE` | `WIDEVINE_SOFTWARE` | issued, key status **usable**; key container level **`SW_SECURE_DECODE`**, no HDCP |
| 720p, 1080p (and 1140–1920 wide), H.264 and HEVC | `TYPE-0` | `WIDEVINE_HARDWARE` | issued, key status **`output-restricted`** (the key requires HDCP, which a software CDM cannot report) |
| 4K HEVC | `TYPE-1` | `WIDEVINE_HARDWARE` | **refused**, licence status `-42604` |
| Every audio rendition (one key) | – | – | issued, **usable**; level `SW_SECURE_CRYPTO` |

So the best a software CDM is licensed for is SD (864×486); no HD.

**Media playlists.** VOD, one `.m4s` file per segment (no byte ranges), `EXT-X-MAP` init file; video segments 7–10 s, audio about 10 s. Three `#EXT-X-KEY:METHOD=SAMPLE-AES` lines (FairPlay `skd://`, PlayReady, and Widevine `data:text/plain;base64,<pssh>`); one key for every SD variant, another for every HD variant, a third for 4K, and one for audio. Encryption is `cbcs` (video pattern 1:9, a 16-byte constant IV, subsamples in `senc`). The video init segment is `encv`→`avc1` with an **all-zero `tenc` key ID**, so the key ID must come from the playlist's pssh (as for radio episodes).

**Keys and the decoder.** The licence answer carries each key's required security level. Audio keys are `SW_SECURE_CRYPTO` ("the CDM decrypts"), which the CDM's `Decrypt` serves, as for songs. Every video key Apple issues to a software CDM is `SW_SECURE_DECODE` ("the CDM decrypts *and decodes*"): the CDM installs the key (status usable) but its `Decrypt` returns `kNoKey` for it (verified: audio samples decrypt, the first video sample does not). Such keys are usable only through the CDM's own video decoder (`InitializeVideoDecoder` and `DecryptAndDecodeFrame` in the Chromium CDM interface), which hands decoded pictures to the host. The maintainer allowed that decoder in the sandboxed CDM host child, and decoded pictures leaving the child for display, in memory only; so a video's audio still goes through `Decrypt` and its video through `DecryptAndDecodeFrame`.

**What Sona plays.** The best H.264 variant with `HDCP-LEVEL=NONE` and `WIDEVINE_SOFTWARE` (`hls.SelectVideo`), 864×486 on 16:9 videos; never HEVC, which this CDM's decoder lacks (it takes H.264, VP8, VP9 and AV1), and never HD (`output-restricted`). Without such a variant the item fails with `video_hd_only`; a refused or unusable video key with `video_licence_refused`. The audio and video keys are licensed separately (two sessions, two renewals). The decoder takes Annex B access units (`internal/playback/avc`), so each sample's length prefixes become start codes, an IDR picture without its own parameter sets gets the `avcC`'s SPS and PPS first, and the `cbcs` subsample map is adjusted so the encrypted bytes keep their place. Apple's renditions start their timestamps at an offset (10 s on the videos checked, the same in audio and video): the engine reads it from the first audio segment and puts the picture on the audio's timeline. `go run ./cmd/playspike -probe-video <id>` prints the licence result per variant; `-video <id> [-seek N]` plays one and checks pictures, sync, seek and the end (without printing the master playlist's URL).

**Showing the picture.** The CDM host child can't draw into Sona's main window: a Wayland subsurface needs the parent's `wl_surface`, which can't be shared across processes. A toplevel window owned by the child (spiked under the sandbox in a headless sway) can't follow the video area of the page, because Wayland lets no client position its toplevels. So the child sends each decoded picture (I420) to the parent when the audio being heard reaches it, the parent keeps only the latest one in memory (`video.Surface`), and the UI's video feed takes it in the same process, converts it to RGBA and draws it as a bitmap (`kit.VideoSurface`). Measured in Sona: first picture about 1.5 s after play, 25 pictures a second, pictures about 20 ms ahead of the audio (on purpose, for the trip to the screen), a seek's first picture in 0.3–0.75 s.

### 4.6 Station (`/{sf}/station/{slug}/{ra.id}`)

Station ID kinds: `ra.<n>` (editorial, live, or radio episode), `ra.<artistId>` (artist station), `ra.<songId>` (song station), `ra.a-<artistId>` (artist "for you" station), `ra.u-…` (the user's personal station), `ra.q-…` (mood or discovery), `ra.cp-<songId>` (AutoPlay "continue playing" station). Data: `name`, `artwork`, `editorialArtwork`, `editorialNotes`, `isLive`, `kind`, `mediaKind`, `radioUrl`, `requiresSubscription`, `stationProviderName`, `supportedDrms`, `airTime` and `durationInMillis` (episodes), and `radio-show` (an apple-curator). The page is minimal: play and Copy Link.

- `GET amp-api /v1/catalog/{sf}/stations/{id}?include=radio-show&extend=editorialArtwork&art[url]=f&format[resources]=map` → **200**, MUT: no (`ra.u-…` also 200 with the MUT; it is personal).
- `GET amp-api /v1/catalog/{sf}/stations?ids=ra.471744,ra.1122782290` → **200**. Used by "Create Station" to resolve artist and song stations.

### 4.7 Curator and Apple curator (`/{sf}/curator/{slug}/{id}`)

One route serves three kinds of `apple-curators`, told apart by `attributes.kind`:

- **Genre**: category pages such as Alternative or Pop.
- **Show**: radio shows such as The Zane Lowe Show. These add `showHostName` and `editorialNotes`.
- **Curator**: brand or editorial.

External curators are the `curators` type.

Data: `name`, `shortName`, `artwork`, `editorialArtwork`, `editorialNotes`, `playlistCount`, plus a `grouping` relationship that gives the page's editorial sections. For a genre: Latest Songs, Playlists, New Releases, Artist Playlists, Stations, Spatial Audio, Videos, decade/year shelves, Essential Albums, Artists We Love, and so on. For a show: Episodes (stations), Video Highlights, Live Performances, Featured Playlists, You Might Also Like.

- `GET amp-api /v1/catalog/{sf}?ids[apple-curators]={id}&ids[curators]={id}&include=grouping,playlists&extend=editorialArtwork,seoDescription,seoTitle&extend[apple-curators]=playlistCount&extend[curators]=playlistCount&fields[albums]=…&art[url]=f&format[resources]=map` → **200**, MUT: no. `apple-curators` (rel: grouping, playlists) → `groupings` → `editorial-elements` → `albums`, `playlists`, `stations`, `songs`, `music-videos`, `rooms`, `artists`. (The web asks for both types at once because the ID's type is unknown from the URL.)
- Public API `GET api /v1/catalog/{sf}/apple-curators/{id}?include=playlists` → **200**, but with no grouping sections.

### 4.8 Grouping, room and multiroom ("See All" and campaign pages)

- **Grouping** `/{sf}/grouping/{id}` (e.g. Music Videos hub): `GET amp-api-edge /v1/editorial/{sf}/groupings/{id}?tabs=subscriber&include[groupings]=curator&include[songs]=artists,albums&include[albums]=artists&include[music-videos]=artists&extend=artistUrl,editorialArtwork,plainEditorialNotes&art[url]=c,f&format[resources]=map` → **200**, MUT: no. Same shape as New.
- **Room** `/{sf}/room/{id}`, the "See All" grid for an editorial section: `GET amp-api-edge /v1/editorial/{sf}/rooms/{id}?extend=offers,seoDescription,seoTitle&extend[albums]=artistUrl&include[albums]=artists,composers&include[songs]=artists,composers&relate[songs]=albums&art[url]=c,f&format[resources]=map`, then pages `…/rooms/{id}/contents?offset=N` → **200**, MUT: no. `rooms{title, resourceTypes, sorts, defaultSort, editorialElementKind, lastModifiedDate}` → contents.
- **Multiroom** `/{sf}/multi-room/{id}` (campaigns such as "Burna Boy: Apple Music Live"): `GET amp-api-edge /v1/editorial/{sf}/multirooms/{id}?extend=editorialArtwork,lockupStyle,seoDescription,seoTitle,uber&art[url]=c,f&format[resources]=map` → **200**, MUT: no. `multirooms{title, uber, lastModifiedDate}` → `children` (`rooms`, `editorial-elements`) → songs, stations, playlists.
- `/{sf}/genre/{id}` redirects to Search on the web. `GET /v1/catalog/{sf}/genres/{id}` → 200 (`genres{name, parentId, parentName}`).
- Record label `/{sf}/label/{name}/{id}` exists in the router. **Unverified:** no label ID surfaced (Parachutes and Moon Music carry no `record-labels` relationship data, and a label search returned nothing). Expected call: `GET amp-api /v1/catalog/{sf}/record-labels/{id}?views=latest-releases,top-releases` `[INFERENCE]`.

### 4.9 Charts (`/{sf}/new/top-charts`, `/{sf}/new/top-charts/{songs|albums|playlists|music-videos|city-charts|daily-global-top-charts}`, `/{genreId}`)

Data: Top Songs, Top Albums, Top Playlists, Top Videos (`chart: most-played`), City Charts ("Top 25 by city", `city-top`, as playlists) and Daily Top 100 by country (`daily-global-top`, as playlists). There is a genre filter (`genre=34` means all music; the genres list comes from `/genres`).

| Call | Status | MUT | Response |
|---|---|---|---|
| Overview: `GET amp-api /v1/catalog/{sf}/charts?types=albums,music-videos,playlists,songs&genre=34&limit=50&with=cityCharts,dailyGlobalTopCharts&include=tracks&include[songs]=artists&include[albums]=artists&include[music-videos]=artists&relate[songs]=albums&extend=artistUrl&art[url]=f&format[resources]=map&omit[resource]=autos` | 200 | no | `results.{songs, albums, playlists, music-videos, cityCharts, dailyGlobalTopCharts}[]` = `{chart, name, data, next}`; `meta.results.order` |
| Paging one chart: `GET …/charts?chart=most-played&types=songs&genre=34&limit=50&offset=50` | 200 | no | `results.songs` |
| City / daily pages: `GET …/charts?chartId=135&genre=34&limit=50&offset=N` (`119` = daily top 100) | 200 | no | `results.cityCharts` (or `dailyGlobalTopCharts`) → playlists |
| Genre list: `GET amp-api /v1/catalog/{sf}/genres?ids[l]=en-GB&ids[platform]=mac` | 200 | no | 38 `genres{name, url}` |

### 4.10 Concerts (`/{sf}/concerts`, `/concerts/{id}`, `/concerts/artist/{id}`)

Data: concerts near a chosen place, filterable by dates and genres; each concert has its venue, its artists and the sellers of its tickets. The data is Shazam's event feed served through amp-api, with the web player's developer token and nothing else: **no MUT is needed** except for the user's own artists, and no other key. Re-checked 2026-11-03 (bundle read plus live replays through `applemusic.Client`, with and without the MUT).

**Where.** Everything is asked for a place as a **geohash** (`geoHashLocation`, 4 characters: a cell about 39 km wide and 20 km high; the web player's own picker also uses 4 and keeps 5 in its recent cities). Without it the hub answers **500**. The storefront in the path (`/v1/concerts/{sf}/hub`) only chooses the language and the URLs, not the place: `us` answers for London, and `xx` is a 400. The results are the concerts within about 100 km of the cell, nearest cells first by Apple's own reckoning; Sona sorts by date and shows each venue's distance from the chosen city.

- `GET amp-api /v1/concerts/{sf}/hub?containers=popular-concerts&limit[popular-concerts]=100&geoHashLocation=gcpv&include[concerts]=venues,artists&extend[concerts]=tickets&fields[artists]=name,artwork&art[url]=f&art[artists:url]=c&omit[resource]=autos&format[resources]=map` → **200**, MUT: no. `results.containers.{name}.{data, href, title}` hold `concerts` references; `resources` hold `concerts`, `venues` and `artists`. Sona asks for this one container (see below) and sends nothing else about the user.
- **Containers** (`containers=a,b`): `popular-concerts`, `concerts-this-week`, `concerts-next-week`, `more-upcoming-concerts`, `favorite-artists-concerts` (MUT: yes; absent without it) and `all-upcoming-concerts` (never seen filled in the hub). The web page asks for six and draws one shelf each. Observed: **`popular-concerts` is every concert in the area, soonest first (despite its name), up to its limit**: `limit[popular-concerts]` up to 1000 works (1.8 MB with tickets), 3000 returns nothing; the default is 30. **There is no paging**: `offset`, `offset[container]` and `next` do nothing, so a longer list is a bigger `limit`. The week containers and `more-upcoming-concerts` ignore the filters (they drop out of the response when one is set); `popular-concerts` and `favorite-artists-concerts` honour them. Titles are localised by the language parameter (`Popular`, `Favourites`, …); Sona words its own.
- **Filters**: `filter[date]=2026-11-10..2026-11-20` (calendar days, both ends, the venue's local days; the web player sends `YYYY-MM-DD` from the user's local dates) and `filter[genre]=20,21` (catalogue genre IDs, the same list as Charts' `/genres`: `chartLabel` or `name`, `34` = all music is dropped). An unknown genre ID is ignored, not an error.
- **Concert** (`concerts`, ID `ce.<uuid>`): `name` ("Artist with Support @ Venue"), `startISODateTime` and `endISODateTime` (UTC instants), `timezone` (the venue's IANA zone: the show's day and clock are read in it), `url` (its music.apple.com page), and with `extend[concerts]=tickets` `tickets[{vendor, url}]`: the seller's page, mostly through an affiliate redirect (`…evy.net/c/…?u=<seller>`, `SharedID=Apple`), which Sona opens as given and never edits. `relationships.artists` and `.venues` (each one entry).
- **Venue** (`venues`, ID `evu.<id>`): `name`, `geoLocation{latitude, longitude}`, `structuredAddress{address, city, region (often null), postCode, country, countryIsoCode}`.
- **Artist's concerts**: `GET amp-api /v1/catalog/{sf}/artists/{id}?views=all-upcoming-concerts,nearby-upcoming-concerts&geoHashLocation={gh}&include[concerts]=venues&extend[concerts]=tickets&fields[artists]=name,artwork&format[resources]=map` → **200**, MUT: no. `resources.artists.{id}.views.{view}.data` are `concerts` references; `nearby-upcoming-concerts` is present only with a geohash, `all-upcoming-concerts` always when the artist has any (an artist with none has no view). The web's artist page asks for the same views inside its canvas request (`include[concerts]=artists,venues`, `extend[concerts]=tickets`, `geoHashLocation`), and Home's recommendations request takes `geoHashLocation` and `include[concerts]=venues,artists` for the "concerts-for-you" shelf; **Sona sends neither**: that shelf is a prompt to set a location on the web, and the user's place is not added to a personal request.
- **Where the web gets a place.** Its location picker searches with **MapKit JS** (`mapkit.Search`, `libraries: ["services"]`) using a JWT embedded in the page for `music.apple.com`, a separate service with its own token and terms, then encodes the result's coordinates as a geohash and remembers the last five in a cookie. Without a place the web page asks `popular-concerts` of several default cities per storefront (a table in the bundle: London, Manchester, … for `gb`; five regional lists elsewhere). **Sona uses neither MapKit nor that token**: it ships its own list of cities (`internal/places`, about 600, with coordinates and a few other names, searched on the computer) and a default for each storefront (its country's main city, else London). Every list city was checked against the hub (all answered; 17 in Ukraine, Russia, Belarus and a few others had no concerts, which the page says plainly).

Sona: `applemusic.ConcertsNear` / `ArtistConcerts`, `ConcertsService` (page, artist shelf, location, picker), the **Concerts** page (`/concerts`), the artist page's "Upcoming concerts" and Home's concerts shelf. The chosen city is a setting (`concerts.city`, a list ID), kept out of settings exports; Apple is asked about its geohash only, and the lists are cached like other catalogue pages.

## 5. Player and actions

### 5.1 Playing items

- **Song / album / playlist:** `POST play.music.apple.com/WebObjects/MZPlay.woa/wa/webPlayback` with body `{"salableAdamId":"<songId>"}` → **200** (`play.itunes.apple.com` returns the same). `songList[0]{assets[{flavor, URL, …}], hls-key-cert-url, hls-key-server-url, widevine-cert-url, songId, is-itunes-stream, artwork-urls}`. Flavors observed: `30:cbcp256, 34:cbcp64, 28:ctrp256, 32:ctrp64, 37:ibhp256, 38:ibhp64` (AAC only). Then `GET widevineCert` and `POST acquireWebPlaybackLicense`. Library tracks send `isLibrary`. **Already in Sona.** For a whole container the web first batch-loads `GET /v1/catalog/{sf}/albums?ids=…&include=tracks` (or `songs?ids=`).
- **Now-playing metadata:** `GET amp-api /v1/catalog/{sf}/songs/{id}?include[songs]=albums,artists&fields[songs]=name,url,hasLyrics&fields[albums]=name,url,editorialVideo&fields[artists]=name,url`.
- **Play activity:** `POST universal-activity-service.itunes.apple.com/play` (`JSPLAY`). **Already in Sona.**
- **Live radio** (Apple Music 1, Hits, Country, Club, Chill): `GET amp-api /v1/play/assets?id=ra.<id>&kind=radioStation&format=stream&hasDrm=true&mediaType=0&keyFormat=web` (`stationHash` is optional) → **200**, MUT: yes (403 without). `results.assets[{url, keyServerUrl: linear.tv.apple.com/v1/radio/streaming-key-delivery, widevineKeyCertificateUrl, fairPlayKeyCertificateUrl}]`. `url` is a master playlist of CMAF AAC variants (`mp4a.40.2` 256k, `mp4a.40.5` 64k, plus a backup CDN); the media playlist is a sliding window of 16 s fMP4 segments, `SAMPLE-AES` (`cbcs`, constant IV, no subsamples, `senc` with zero entries) with three `EXT-X-KEY`s per programme (PlayReady, Widevine `data:` URI holding a whole pssh box, FairPlay `skd://`); each programme (`#EXT-X-DISCONTINUITY`) brings a new init segment and key. **Licence (verified in Sona):** `POST keyServerUrl` with headers as for songs plus `X-Apple-Renewal: true` and body `{challenge, uri: <the Widevine data URI>, "key-system": "com.widevine.alpha", event: "beats1", stkn?}` (MusicKit's live-station shape; `amtv` is live video) → `{status: 0, license}`; Sona plays Apple Music 1 through the CDM host. Sona requests the asset with the station's own `playParams` (looked up in the catalog), as MusicKit does.
- **Local broadcasters** (the Radio grouping's "Local Broadcasters" element, kind `326`, and "International Broadcasters"; checked 2026-11-01 in gb, us, de, au): stations with `playParams{format: stream, hasDrm: false}`, `isLive: true`, either without `streamingKind` (Capital, KISS, Classic FM, …) or `streamingKind: 2` with `stationProviderName: "TuneIn"` (Heart, Smooth; most of us and de). `GET amp-api /v1/play/assets?…&hasDrm=false…` → **200**, MUT: yes (403 without); `hasDrm=true` → 404. `results.assets[{url}]` only: no key server, no certificate. The URL is the broadcaster's clear stream: Global (`hls.musicradio.com`, plain `http`) an HLS playlist of MPEG-TS segments (ADTS AAC-LC ~53 kbps; a timed-ID3 PID that stayed empty for 7 minutes; no EXTINF titles); Bauer/Rayo (`stream.planetradio.co.uk` → `hls-kiss.hellorayo.co.uk`) HLS of packed ADTS HE-AAC segments, each starting with an ID3v2.4 tag (`TIT2`, `TPE1`, `WXXX` event URL) mirrored in `#EXTINF:10,title="…",artist="…",url="…"`, with title and artist swapped by the broadcaster, "AD n" during adverts and empty fields between songs; TuneIn a PLS (`pls.tunein.com/pre/v1/playlist.pls`, `audio/x-scpls`) listing a station ident and a pre-roll, then the Icecast MP3 stream (128k, `icy-metaint: 8000`, `StreamTitle='Artist - Title'`) ten times over. No schedule (`events` empty). TuneIn's PLS URL carries the station: its `ti_ad_params` query value is itself a query string with `aw_0_1st.stationId=s…` (the TuneIn guide ID; checked 2026-11-03 for Heart UK and Smooth UK in gb), which `opml.radiotime.com/Describe.ashx?id=s…&render=json` answers with the station's profile (`name`, `slogan`, `description`, `genre_name`, `location`, `url`, `has_song`, and `current_song` / `current_artist` for the few stations that feed TuneIn what they play; a station it does not know answers HTTP 200 with `head.status` "400"). `Describe.ashx?c=nowplaying` adds the show on air but its song is an unlabelled text line, and `api.tunein.com/profiles/<id>/nowPlaying` is TuneIn's own app API; Sona uses neither. The web player does not play these: its `isBroadcastRadio` (streamingKind 2, or live without streamingKind and `hasDrm: false`) removes the play action, so it reports no play activity for them. Sona plays them in the parent (`internal/playback/broadcast`, fed to the host as a live file) and, as for every `ra.` item, reports no play activity.
- **Algorithmic stations** (artist, song, album, personal, mood, genre): `POST amp-api /v1/me/stations/next-tracks/{ra.id}?limit=10` (no body; GET → 405) → **200**, MUT: yes. `data` = 10 `songs`; `relate[songs]=albums,artists` adds the album and artist IDs. Call it again for more. Also 200 on `api.music.apple.com`. Each track then plays via `webPlayback`. Album stations exist (`ra.<albumId>`, `kind: albumSeeded`).
- **Radio episodes** (`ra.<n>` with `airTime`, `durationInMillis`, `playParams{format: stream, streamingKind: 1}`): the same `play/assets` path and radio licence (`event: "beats1"` accepted) as live radio, but a VOD playlist; their init segments carry an **all-zero `tenc` default key ID**, so the key ID must come from the playlist's pssh. Verified playing and seeking in Sona.
- **Library uploads** (`i.…` without a catalog song): `webPlayback` with `{"universalLibraryId":"i.…"}` returns `songList[0]{assets, needsPlaybackReporting, songId: -1}` with one flavorless asset whose `URL` is the uploaded file itself on `store-0xx.blobstore.apple.com` (unencrypted MP3/AAC/ALAC, served with Range support; `metadata` adds `cloud-id`, `comments`, `dateAdded`). No licence. Sona's parent downloads it and streams the bytes to the playback host, which decodes them with `decodebin`.
- **Loudness (Sound Check).** Apple measures every catalog song and stores it in the **init segment**, in an ISO/IEC 14496-12 loudness box: `moov/trak/udta/ludt/tlou` (track) and `alou` (album; absent on some old assets). Encoding (ISO/IEC 23003-4): FullBox v1, `loudness_base_count`; per set `EQ_set_ID`, `downmix_ID`/`DRC_set_ID`, `sample_peak(12)`/`true_peak(12)` coded as `20 − v/32` dB, then measurements `{method_definition, method_value, system/reliability}`; method 1 is BS.1770 integrated loudness, `−57.75 + v/4` LUFS. Example, "Enter Sandman": −12.00 LUFS, true peak −0.94 dBTP. `tlou` is identical in ctrp256, ctrp64 and ALAC inits; `alou` differs per album asset. **Nowhere else:** not in webPlayback (`assets[].metadata` has bitrate, gapless, sample rate but no loudness), not in the HLS playlists (the enhanced-HLS master's `com.apple.hls.audioAssetMetadata` session data has format fields only), not in catalog attributes, and MusicKit JS v3, Apple's hls.js and the music.apple.com bundles apply no normalisation at all (no `soundcheck`/`loudness`/`ludt` references). Catalog preview `.m4a` files carry an iTunes `iTunNORM` tag; uploads carry `iTunNORM` only if iTunes/Music wrote one into the file (`moov/udta/meta/ilst/----`, or an ID3 `COMM` frame in MP3). Sona's Sound Check uses `tlou` (target −16 LUFS, raises capped at −1 dBTP true peak) and an upload's own `iTunNORM`.

### 5.2 Queue ("Up Next"), shuffle, repeat, AutoPlay

- The web shows **only upcoming items** (no History list). Each row can be removed, and there is Clear. When AutoPlay is on, the header says "autoplay is on". Transport offers shuffle, previous, play/pause, next, repeat (off/all/one) and volume. Sona already has a local queue (Play Next, Append, Remove, Move, Clear, Jump) plus shuffle and repeat, and its own history.
- **AutoPlay** (the ∞ toggle; "Similar music will keep playing"):
  1. `POST amp-api /v1/me/stations/continuous` with body `{"data":[{"id":"<seed songId>","type":"songs","meta":{"container":{"id":"<container id>"}}}]}`. Library seeds use `"type":"library-songs"`, and the seed can be several items. The optional query `limit[results:tracks]=N&with=tracks` returns tracks inline. Result: **200**, MUT: yes. `results.station` = `stations{id: "ra.cp-<songId>", name, artwork, kind, …}`.
  2. `POST amp-api /v1/me/stations/next-tracks/ra.cp-<songId>?limit=10` → **200**, 10 `songs`. These are appended after the user's queue and refilled as it drains.
  Verified live with the UI capture and the probe.
  The web player's Up Next lists the AutoPlay songs in their own list under the "AutoPlay" heading (`ASE.Web.Music.UpNext.Autoplay.Heading`) with "Similar music will keep playing" under it (`ASE.Web.Music.UpNext.Autoplay.Description`); its header says "autoplay is on" (`ASE.Web.Music.UpNext.Heading.AutoplayActive`). Checked in its bundle: the user's rows are drop targets and the AutoPlay rows are not, so nothing can be dropped among or move into the AutoPlay songs, but each AutoPlay row keeps its remove button and "Remove item from Up Next" (`ASE.Web.Music.UpNext.RemoveItem`), and a double-click plays it (a jump to that queue position). Sona does the same, sets the AutoPlay rows back in grey, and fetches the AutoPlay batch ahead (`docs/architecture.md` §4.6).

### 5.3 Lyrics

- `GET amp-api /v1/catalog/{sf}/songs/{id}/syllable-lyrics?l[lyrics]=en-gb&l[script]=en-Latn&extend=ttmlLocalizations` → **200**, MUT: **yes** (404 without). `syllable-lyrics{ttmlLocalizations, playParams}`: TTML with `itunes:timing="Line"` or `"Word"`, `<translations>` / transliterations (time-synced translation and pronunciation, toggled from the lyrics menu), songwriters and `ttm:agent` (singer parts: each `<p>` names one, declared in the head as `<ttm:agent type="person|group|other" xml:id="v1">`; `group` is every voice together), and background vocals as `<span ttm:role="x-bg">` inside a line.
- `GET amp-api /v1/catalog/{sf}/songs/{id}/lyrics` → **200**, `lyrics{ttml}` (line-timed fallback).
- `songs.attributes.hasLyrics` / `hasTimeSyncedLyrics` decide whether lyrics are available. `lyricsExcerpt` gives the song page excerpt.
- `GET api …/lyrics` → 400. Sona uses amp-api: `syllable-lyrics` with the `l[lyrics]`/`l[script]`/`extend=ttmlLocalizations` query, then `lyrics` (`internal/lyrics`'s Apple provider, one of the lyrics sources in Settings → Playback → Lyrics).
- The bundle also has `/v1/lyrics/{sf}/snippet/songs/{id}` (lyric share snippet). Unverified, and not needed.

### 5.4 Create Station

From a song, playlist track or artist menu, the web resolves the station and starts it:

1. `GET amp-api /v1/catalog/{sf}/stations?ids=ra.<songId|artistId>` → 200.
2. `POST /v1/me/stations/next-tracks/ra.<id>` → 200.

Both were captured live (artist `ra.471744`, song `ra.6800467168`).

### 5.5 Rating model: Favourite, Suggest Less (replaces Love/Dislike)

The current web has **no Love/Dislike wording**. The actions are **Favourite / Undo Favourite** and **Suggest Less / Undo Suggest Less**, for songs, albums, playlists, music videos, artists and library playlists.

| Action | Call | Verified effect |
|---|---|---|
| Favourite | `POST amp-api /v1/me/favorites?ids[songs]={id}` (also `ids[albums]`, `ids[artists]`, `ids[playlists]`, `ids[music-videos]`), no body | 202 (web) / 2xx (probe). **Sets rating = 1, and for songs also adds the song to the library** (a library copy appeared and stayed after un-favouriting). Artist: rating 1 only. |
| Undo Favourite | `DELETE amp-api /v1/me/favorites?ids[songs]={id}` | 204. Rating removed; the library copy **is not** removed. |
| Suggest Less | `PUT amp-api /v1/me/ratings/{songs|albums|playlists|artists|music-videos|library-songs|…}/{id}` with body `{"type":"rating","attributes":{"value":-1}}` | 200 → `ratings{value:-1}` |
| Undo Suggest Less | `DELETE amp-api /v1/me/ratings/{type}/{id}` `[bundle]` | 2xx |
| Read state | `GET amp-api /v1/me/ratings/{type}?ids=a,b,…` (also `fields[…]=personalRating` in library-state hydration) | 200 → `ratings{value: 1|-1}`; `{"data":[]}` when unrated |
| Legacy "love" | `PUT …/ratings/{type}/{id}` with `value: 1` | 200. Shows as Favourite on the web, but **does not** add to the library. |

Sona has `SetRating`/`DeleteRating`/`Ratings` (`/v1/me/ratings`). It lacks `/v1/me/favorites` and its add-to-library side effect, which Favourite needs to match the web. The **Favourite Songs** playlist is a library playlist tagged `favorited`.

### 5.6 Context menus (item type → actions, observed)

| Item | Actions |
|---|---|
| Catalog song (album or playlist row) | Add to Library, Add to Playlist ▸ (New Playlist, suggested/recent playlists, folder tree), Play Next, Play Last, Create Station, Favourite, Suggest Less, View Credits, Copy Link, Copy Embed Code |
| Song already in library | Pin Song, Delete from Library, Add to Playlist ▸, Play Next, Play Last, Create Station, Favourite / Undo Favourite, Suggest Less / Undo Suggest Less, View Credits / Show Album, Copy Link, Copy Embed Code |
| Track inside a user playlist | Remove from Playlist, Pin Song, Delete from Library, plus the above |
| Album / catalog playlist / music video (header) | Add to Library, Add to Playlist ▸, Play Next, Play Last, Favourite, Suggest Less, Copy Link, Copy Embed Code |
| Library playlist (header) | Edit, Pin Playlist, Delete from Library, Add to Playlist ▸, Play Next, Play Last, Favourite, Suggest Less, Copy Link, Copy Embed Code |
| Artist | Create Station, Favourite, Suggest Less, Copy Link |
| Station | Copy Link |
| Account button | Help, Settings, Transfer Music, Sign Out |

"Copy Link" copies the item's `attributes.url` (music.apple.com). "Copy Embed Code" builds an `embed.music.apple.com` iframe; it is web-only and optional for Sona. "View Credits" opens the song credits (see §4.4). Delete from Library asks for confirmation ("…also remove it from any playlists").

### 5.7 Library mutations

| Action | Call | Status / notes |
|---|---|---|
| Add to library | `POST amp-api /v1/me/library?ids[songs]=…` (also `ids[albums]`, `ids[playlists]`, `ids[music-videos]`) `&representation=ids&format[resources]=map` | 200. `data` = new `library-songs` IDs; `meta.operation.songs[<catalogId>]` maps each catalog ID to its library ID. (Sona has `AddToLibrary` without `representation`, where the body is ignored.) |
| Pre-add (album not out yet) | The same `POST /v1/me/library?ids[albums]=…&representation=ids`: the web player's bundle (checked 2026-11-01) has no separate pre-add call; `requestAddToLibrary` serves both, and it treats an album as pre-release when its `releaseDate` is after now (`isPreReleaseAlbum`). MUT: yes. | Sona: `LibraryService.Add`, labelled "Pre-add" on an upcoming album. A pre-release album's page lists every track; only the songs already out carry `playParams` and their own `releaseDate`, the rest are named "Track N" without either. The album page answer (`include=tracks`) may omit `isPrerelease`, so Sona goes by `releaseDate` (local midnight). The library side of a pre-added album: §3.1. The en-GB web strings have "Pre-Release" and "COMING {releaseDate}" (upper case), and nothing for pre-add or reminders. |
| Delete from library | `DELETE amp-api /v1/me/library/{songs|albums|playlists|music-videos|artists}/{libraryId}` | 204. Needs the **library** ID; get it from `relate=library`. Not in Sona. |
| Pin | `POST amp-api /v1/me/library/pins/{libraryId}` (no body) | 2xx. New pin action defaults to `play`. The web treats 400 as "max pins reached"; GET is capped at `limit=6`. |
| Pin action | `PATCH amp-api /v1/me/library/pins/{libraryId}?action=play|drillIn|shuffle` | 2xx. Verified that `meta.libraryPin.action` changed. |
| Move pin | `POST amp-api /v1/me/library/pins/{libraryId}?after={otherId|top}` | 2xx. Verified the pin moved first. |
| Unpin | `DELETE amp-api /v1/me/library/pins/{libraryId}` | 2xx. Verified it was removed. |

### 5.8 Playlist editing

All on `amp-api` with the MUT required. Each was verified on a temporary playlist that was then deleted.

| Action | Call | Status / notes |
|---|---|---|
| Create | `POST /v1/me/library/playlists` with body `{"attributes":{"name","description","isPublic"},"relationships":{"tracks":{"data":[{"id","type":"songs"|"library-songs"|"music-videos"}]}}}` (`?with=shared` when public) | 200. `data[library-playlists]{canDelete, canEdit, description.standard, isPublic, …}`. The web always creates with ≥1 track (from "Add to Playlist ▸ New Playlist"). |
| Rename / description / visibility ("Edit") | `PATCH /v1/me/library/playlists/{id}` with body `{"attributes":{"name","description","isPublic"}}` | 2xx. Verified name and `description.standard` changed. |
| Add tracks | `POST /v1/me/library/playlists/{id}/tracks?representation=resources` with body `{"data":[{"id","type"}]}` | 200. Returns `library-songs` plus `meta.operation`. **Side effect: added catalog songs also become library songs.** |
| Reorder / replace | `PUT /v1/me/library/playlists/{id}/tracks` with body `{"data":[{"id":"i.…","type":"library-songs"},…]}` (full ordered list) | 2xx. Verified the new order. |
| Remove a track | `DELETE /v1/me/library/playlists/{id}/tracks?ids[library-songs]={i.id}&mode=all` | 2xx. Verified. (`mode=all` removes every occurrence.) |
| Move to folder | `PUT /v1/me/library/playlists/{id}/parent` with body `{"data":[{"id":"<folderId>","type":"library-playlist-folders"}]}` | 2xx. The folder listing did not show the playlist within a few seconds (eventual consistency or the folder's paging; `[INFERENCE]`). |
| Cover (artwork) | **None.** Apple's web API cannot set or upload a library playlist's artwork; see "Playlist artwork" below. | Sona keeps the cover itself (`internal/library/covers.go`). |
| Delete | `DELETE /v1/me/library/playlists/{id}` | 2xx. Afterwards `GET` still returns a **tombstone** (`canDelete:false`, no `name`, `lastModifiedDate: 1970-01-01`). Sync must drop these. |
| Create / rename / delete folder | Not in the web bundle (the web only lists folders and moves playlists into them). | **Unverified**, to avoid leaving artefacts in a real library. Folders show `canEdit`/`canDelete`. |
| Collaboration | Playlist `views=collaborators,contributors,subscribers` and `extend=hasCollaboration,isCollaborativeHost`; the web shows them read-only. A library copy of someone else's collaborative playlist has `hasCollaboration: true`, `isCollaborativeHost: false`, `canEdit: false`. Who the viewer is: their profile ID from `GET /v1/me/social/profile?include=social-profile` against the collaborators (Sona: owner, collaborator or listener). | Read verified (200). Join/approve endpoints were not seen in the web UI. |

#### Playlist artwork: not settable through the web API (checked 2026-11-05)

Question: can a client choose or upload a library playlist's cover, as the Apple Music apps do? Answer: not with the web player's token and API, so Sona stores a user's choice itself (`docs/architecture.md`, "Artwork", and `docs/design.md`, "Artwork").

- **The web player has no such feature.** The playlist editor's update intent (`UpdateLibraryPlaylistIntent` in `index~….js`, the entry script, and its 40 lazily loaded chunks, all read in full) sends only `PATCH /v1/me/library/playlists/{id}` with `{"attributes":{"name","description","isPublic"}}`, and no chunk has a file input or an image reader for playlists. A playlist that has no artwork gets a cover the player makes from its tracks' artwork in the browser (its bundle logs "Failed to load tracks for playlist to populate artwork").
- **Apple generates a cover for every playlist that has a track** (checked 2026-11-05 on a temporary playlist with one song, deleted afterwards). `attributes.artwork` is `{"hasP3":false,"height":1200,"width":1200,"url":"https://is1-ssl.mzstatic.com/image/thumb/gen/1200x1200AM.PDCXS11.jpg?c1=F2CC9D&c2=FFA940&c3=B36B00&c4=400000&signature=…&t=<base64 name>&tc=000000&vkey=1"}`: a diagonal gradient of four colours (`c1` light top left to `c4` darkest bottom right, drawn from the first song's artwork) with the playlist's name at the top left in black (`tc`). It is the same with `art[url]=f` or `c,f` and without. A playlist with no tracks has no `artwork` attribute at all. The URL is fixed-size, but the CDN fills any size with the signature intact (`300x300`, `600x600` and `1200x1200` all answered 200, 12, 33 and 85 kB), so `applemusic.Artwork.Template` turns it into a `{w}x{h}` template and Sona asks for the size it shows. The same `gen/` path serves the initials images of credited people (`AM.SCM01`, `name=`), which Sona drops as not being pictures. This cover is what Sona's *Automatic* choice shows; the mosaic is only for a playlist Apple gave nothing (an empty one, or tracks with no artwork). The name is baked in by Apple: a rename gives a new URL at the next sync.
- **The designs of the iPhone and Mac apps' cover picker are this URL** (checked 2026-11-06 on a real library). A playlist whose cover was set to one of the patterns in the iPhone app comes back with the same `gen/` URL and another design in its path: `AM.PDCXS12` (a tilted disc), `PDCXS14` (rising bars), `PDCXS15`, `PDCXS18` (a grid of circles), `PDCXS19`; `PDCXS11` is the plain gradient a new playlist gets. The design is outside the signature: on one playlist's URL every design from `PDCXS11` to `PDCXS19` answered 200 with that design in the playlist's colours and name (`PDCXS01`–`PDCXS09` are older renderings of the same nine; `PDCXS00`, `PDCXS10` and `PDCXS20` and above answer 404), while a changed `t` (name), `c1` or `tc`, or no `signature`, answers 400. So a client can show a playlist in any of Apple's designs from the URL Apple gave it, but cannot draw one for a playlist Apple gave no generated cover (an empty one, or one with a photo chosen in Apple's apps, which comes back as an ordinary `{w}x{h}` artwork URL). Setting the design on Apple's side is the same as setting a photo: not possible through this API (below). Sona offers the nine designs from the playlist's own URL (`artwork.Patterns`, `artwork.WithPattern`) and keeps the choice locally.
- **The generated covers need `*/*` in `Accept`** (checked 2026-11-06). A size of a `gen/` URL that is not in the CDN's cache is drawn by an origin that answers **406** (`application/problem+json`) when `Accept` lists only image types (`image/jpeg`, `image/png`, `image/*`, or `image/jpeg,image/png;q=0.9`) and 200 `image/jpeg` once `*/*` is in it (`*/*`, `image/jpeg,*/*;q=0.8`, a browser's). A cached size answers 200 whatever `Accept` says, which is why the 1200 px URL Apple's apps fetch worked and Sona's own sizes did not until `artwork.Cache` sent `*/*` (0.3.0-rc.2: every generated playlist cover failed, only mosaics showed). Album artwork (`{w}x{h}bb.jpg`, `cc`, `SC.DN02`) is served with or without it.
- **Live check on a temporary library playlist** (created, probed and deleted; `platform=web`, MUT, nothing logged):
  - A new playlist carries no `artwork` attribute at all (`canDelete`, `canEdit`, `dateAdded`, `description`, `hasCatalog`, `isPublic`, `lastModifiedDate`, `name`, `playParams`).
  - `PATCH /v1/me/library/playlists/{id}` with `attributes.artwork` as `{"url":"data:image/png;base64,…"}`, as a bare base64 string, as `attributes.artworkData`, and as `{"url":"https://is1-ssl.mzstatic.com/…/{w}x{h}bb.jpg","width":600,"height":600}` each answered **2xx and changed nothing**: Apple ignores the unknown attributes (a later `GET` has no `artwork`; a `description` sent beside them was stored).
  - `PATCH` with `relationships.artwork` → **400** `Invalid playlist data` (`40007`).
  - `PUT`, `POST`, `PATCH` and `DELETE` on `/v1/me/library/playlists/{id}/artwork` → **405**; `GET` there → **400** `No relationship found matching 'artwork'` (`40008`).
- The Mac and iOS apps set a cover through the iCloud Music Library's own protocol, not `amp-api`; it is out of reach of a web-token client and is not attempted.

Sona's covers are therefore local (`playlist_covers`, migrations 00010 and 00011): the choice is shown on every Sona surface on this computer and is not synced to Apple or to Sona on another device. The picker says so.

### 5.9 Badges and explicit content

- **Explicit:** `attributes.contentRating == "explicit"` (`"clean"` for clean versions) on songs, albums, music videos and playlists. The web adds an "Explicit" label to the accessible name.
- **Restricted accounts:** `GET /v1/me/account` → `attributes.restrictions` (`{}` when unrestricted) and the `itre` (restrictions enabled) cookie. Storefront `explicitContentPolicy` (`allowed` or not). `[INFERENCE]` When restricted, explicit items should be hidden or disabled; not yet observed with a restricted account.
- **Audio quality:** `audioTraits` (`lossless`, `hi-res-lossless`, `lossy-stereo`, `atmos`, `spatial`), `isAppleDigitalMaster`, `isMasteredForItunes`. The web player only receives AAC flavors (`cbcp256`/`ctrp256`/`ibhp256`), so these are **display badges only** for a web-token client like Sona. `extend=extendedAssetUrls` exposes an `enhancedHls` master with ALAC variants and Widevine keys, but Apple's licence server refuses the ALAC key to Chrome's desktop CDM (`-1002`). Lossless, Hi-Res and Atmos playback are not available on this path. `go run ./cmd/playspike -probe-lossless <song>` re-checks every variant key and prints the table. Re-checked 2026-11-01 with the CDM Sona downloads itself (`cdm source source=download version=4.10.3112.0`, Linux x64) on "bad guy" (1450695739, `lossless`, `atmos`): every AAC key (`c22`, `c24`) and the Atmos variants' `c24` are issued and usable; the ALAC 24-bit variant's `c23` is still `REJECTED status -1002`, so the downloaded CDM changes nothing for lossless.
- **Dolby Atmos in `enhancedHls`** (CDM 4.10.3112.0): Atmos tracks list `audio-atmos-2768` and `audio-atmos-2448` (`CODECS="ec-3"`, ~770 and ~450 kbit/s) next to AAC `-binaural`/`-downmix` and ALAC 24-bit groups. "bad guy" (1450695739): the Atmos variants share track key `c24` with the AAC binaural/downmix variants, and Apple **issues** that key to Sona's CDM (licence issued, key usable); ALAC `c23` is rejected (`-1002`) as for stereo lossless. "Blinding Lights" (1488408568): every `enhancedHls` variant, Atmos included, is FairPlay-only (`skd://` keys, no Widevine `EXT-X-KEY`). So Atmos is licensable for some tracks only; playing it would need cbcs E-AC-3 decryption and `avdec_eac3` downmixing (not built; the decrypt/decode of the Atmos segments is untested [INFERENCE: the issued `c24` key decrypts them, as it is the variant's only Widevine key]).
- **Video:** `has4K`, `hasHDR`, `videoTraits`.
- **Other:** `isVocalAttenuationAllowed` (Apple Music Sing; not on the web), `isPrerelease` (Coming Soon), `previews` (30-second clips; usable for non-subscribers).

## 6. Account, profile and social

| Feature | Route | Call | Status | MUT | Applicability |
|---|---|---|---|---|---|
| Own social profile | sidebar account button | `GET /v1/me/social/profile?include=social-profile` | 200 | yes | Show name and avatar (`/v1/me/account` `avatarArtwork`) |
| Public profile page | `/profile/{handle}` | `GET amp-api /v1/social/{sf}/social-profiles?filter[handle]={handle}` and `…/social-profiles?ids=sp.…&extend=followState&art[url]=c,f` (both verified). The page call adds `include=followers,followees,shared-playlists,listening-to` `[bundle, unverified]`. Own profile: `GET /v1/me/social/profile?include=followers,followees,pending-followers,shared-playlists,listening-to&extend=pendingFollowersCount,followState` `[bundle, unverified]` | 200 | no | `social-profiles{name, handle, artwork, isPrivate, isVerified, followState, url}`. The page reads the `shared-playlists` (public playlists), `listening-to`, `followers` and `followees` relationships; it uses no `views`. Private profiles show only the header. |
| Followers / following | profile | `GET /v1/me/social/profile/followers`, `…/followees` | **400** `Invalid Request Signature` | yes | Requires a signed request the web token can't produce. **Not feasible.** |
| Follow / requests / block | profile | `…/followees`, `…/pending-followees` (DELETE cancels a request), `…/blocked-profiles` `[bundle]` | unverified (social-graph mutations not exercised) | yes | Low priority |
| Shared playlists on profile | profile edit | `PUT /v1/me/social/profile/shared-playlists` with body `{"data":[{"id":"p.…","type":"library-playlists"}]}` `[bundle]`. `GET` → 404 `No related resources` for this user | GET 404 (empty) | yes | Low priority |
| Friends listening | Home shelf ("Friends Are Listening To") | part of the Home call (`with=social`) | 200 | yes | Comes with Home |
| Settings | `/{sf}/account/settings` | Embedded Apple ID commerce web flow (`/includes/commerce/…`, idmsa sign-in iframe). It needs an Apple ID web session, not the MUT. | — | — | **Web-only / not applicable.** Sona has its own settings. |
| Subscriptions / billing / create account | `/account/*` | commerce flows | — | — | Not applicable; link out to Apple |
| Transfer Music | `/transfer-music` | `/v1/me/library/import/jobs/{id}/progress` `[bundle]` | unverified | yes | Out of scope |
| Replay | Home shelf + `replay.music.apple.com` | Replay playlists are ordinary catalog playlists `pl.rp-…` (200) | 200 | yes | Show the playlists. The Replay microsite is web-only. |
| Taste onboarding | first run | `POST /v1/me/taste/taste-preferences` `[bundle]` | unverified (would change recommendations) | yes | Optional |
| Report a concern | lyrics/item menu | `POST /v1/catalog/{sf}/concerns` `[bundle]` | unverified (would file a real report) | yes | Optional |
| Region / footer storefront list | footer | static | — | — | Not applicable; Sona uses the account storefront |

## 7. Unverified or not exercised (and why)

- **Folder create/rename/delete** (`/v1/me/library/playlist-folders`): absent from the web bundle. Not attempted, to avoid leaving folders in a real library.
- **Social graph mutations** (follow, unfollow, pending, blocked, shared-playlists PUT): these would alter a real public profile. The read endpoints for followers and followees fail with 400 (signature).
- **Taste preferences, report-a-concern, library import jobs, lyric snippets:** not needed for core features, and mutating ones would have real side effects.
- **Record-label page:** no label ID was available in the observed session.
- **Radio-episode and show playback path:** inferred to be next-tracks or play/assets depending on the station kind. Not driven.
- **Restricted (explicit-filtered) accounts:** not yet observed.
