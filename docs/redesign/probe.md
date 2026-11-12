# Albums scroll probe

The `probe` build (`-tags probe,sqlite_fts5`) scrolled the Albums page in `headless` on the fixture profile (24 albums), with the fixture covers served locally (`SONA_E2E_CDN`). The settings are `-runs 9 -speed 500`, and the window is 1132×1253 on llvmpipe, so CPU figures are not a GPU's. Each probe was run three times.

- **p95** is the table's frame p95: the 95th percentile of the gaps between frames, as the median over the passes.
- **Worst** is the median over the passes of each pass's largest gap (stderr's per-pass `max`). The single largest gap of a run varies from 20 to 54 ms between identical runs, so it can't be compared.

## Baseline (`origin/main`, b259a43)

| Run | p95 | Worst | Dropped |
| --- | --- | --- | --- |
| 1 | 17.2 ms | 19.8 ms | 0 of 148 |
| 2 | 17.1 ms | 18.6 ms | 0 of 148 |
| 3 | 17.0 ms | 17.7 ms | 0 of 149 |
| Mean | 17.1 ms | 18.7 ms | |

Budget (+5%): p95 ≤ 17.96 ms, worst ≤ 19.6 ms.
