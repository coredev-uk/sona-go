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

## After the redesign

The machine was shared with other work while these ran, and a busy moment inflates single runs: run 1 of the first set reads 23.6 ms p95, and its next run 19.8 ms.

First set, the redesign's head alone:

| Run | p95 | Worst | Dropped |
| --- | --- | --- | --- |
| 1 | 23.6 ms | 28.3 ms | 3 of 137 |
| 2 | 19.8 ms | 24.8 ms | 0 of 137 |
| 3 | 17.1 ms | 17.7 ms | 0 of 145 |

So the comparison that counts alternates the two builds (`bin/sona-probe-base` from b259a43, `bin/sona-probe` from this branch), five pairs, so that the same load falls on both:

| Pair | Base p95 | Base worst | Redesign p95 | Redesign worst |
| --- | --- | --- | --- | --- |
| 1 | 23.4 ms | 34.1 ms | 17.1 ms | 18.3 ms |
| 2 | 17.2 ms | 18.2 ms | 17.1 ms | 17.8 ms |
| 3 | 33.2 ms | 47.6 ms | 16.9 ms | 17.4 ms |
| 4 | 17.0 ms | 17.7 ms | 22.4 ms | 35.4 ms |
| 5 | 17.0 ms | 17.7 ms | 17.2 ms | 17.8 ms |
| **Median** | **17.2 ms** | **18.2 ms** | **17.1 ms** | **17.8 ms** |

- **Against the budget:** the redesign's medians are 17.1 ms p95 and 17.8 ms worst, both under the budget of 17.96 ms and 19.6 ms.
- **Against the interleaved base:** they are also no worse than the base build's 17.2 ms and 18.2 ms.
- **CPU work per frame:** the median is 1.30 ms for both builds.
