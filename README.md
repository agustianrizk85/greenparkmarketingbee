# Green Park — Marketing Flow (Backend)

Mesin **alur kerja Departemen Marketing**: setiap konten / campaign berjalan
melalui salah satu dari **4 alur** sesuai flowchart departemen, dengan PIC per
langkah, gate approval Kepala Departemen, pengajuan budget iklan, SLA per
langkah, dan early-warning.

Arsitektur **mengikuti pola `greenparkpermit`**: Go (Gin + GORM), berlapis
`model → repository → service → handler`, mendukung **SQLite** (run cepat) &
**PostgreSQL** (produksi) via `DB_DRIVER`. Seluruh engine (status, budget,
approval, SLA, warning) generik dan **data-driven** dari `service/catalog.go`.

```
backend/
├── cmd/server/main.go
└── internal/
    ├── config database model repository service handler middleware dto seed
    └── service/catalog.go   # definisi 4 alur A–D (sumber kebenaran)
```

## Empat alur (mirror flowchart)

| Alur | Nama | Langkah |
|---|---|---|
| **A** | Iklan Berbayar — Konten Hardsell / Desain Statis | A1–A10 |
| **B** | Iklan Berbayar — Konten Video | B1–B12 |
| **C** | Konten Organik — Carousel | C1–C9 |
| **D** | Konten Organik — Video / Reels | D1–D11 |

Setiap langkah membawa: **Owner** (PIC: Copywriter, Design Grafis, Videografer,
Video Editor, Talent, Social Media Specialist, Digital Marketing, Kepala
Departemen), **Phase** (brief → produksi → review → approval → distribusi),
flag **IsApproval** (gate ke `kadep`), **RequiresBudget** (biaya iklan / top up),
**CollabDept** (Perencanaan / Keuangan / Sales), dan **SLA**.

## Menjalankan

### SQLite (langsung jalan, tanpa setup)
```bash
cd backend
# .env sudah di-set DB_DRIVER=sqlite
go run ./cmd/server            # API :8086, file DB: backend/marketingflow.db
```

### PostgreSQL
Set di `.env`: `DB_DRIVER=postgres` + kredensial DB, lalu `go run ./cmd/server`.

Akun seed otomatis — satu per peran di flowchart (ganti setelah login). Role
`kadep` = approver; `staff` = tim internal (boleh edit langkah); `viewer` = read-only.
Kolom **Position** cocok dengan `WorkStep.Owner`.

| Email | Role | Position | Password default |
|---|---|---|---|
| kadep@greenpark.id | kadep | Kepala Departemen Marketing | `kadep123` |
| copywriter@greenpark.id | staff | Copywriter | `staff123` |
| talent@greenpark.id | staff | Talent | `staff123` |
| videografer@greenpark.id | staff | Videografer | `staff123` |
| editor@greenpark.id | staff | Video Editor | `staff123` |
| design@greenpark.id | staff | Design Grafis | `staff123` |
| sosmed@greenpark.id | staff | Social Media Specialist | `staff123` |
| digital@greenpark.id | staff | Digital Marketing | `staff123` |
| viewer@greenpark.id | viewer | Viewer | `viewer123` |

## API utama

| Method | Endpoint | Keterangan |
|---|---|---|
| POST | `/api/auth/login` · GET `/api/auth/me` | Auth JWT |
| GET | `/api/meta/alur` | Label alur A–D |
| GET/POST | `/api/work-items` | List / buat konten (pilih alur → auto-seed langkah) |
| GET | `/api/work-items/:id` · `/api/work-items/:id/progress` | Detail + progres |
| GET/PUT | `/api/steps/:id` | Detail / update (status, budget, catatan, metadata, SLA) |
| POST | `/api/steps/:id/documents` | Upload lampiran (multipart `file`, `doc_type`) |
| GET | `/api/documents/:id/download` | Unduh lampiran |
| GET | `/api/dashboard/warnings` | Early warning (SLA & budget) |

### Aturan yang ditegakkan
- **Approval** (`is_approval`): hanya role `kadep` yang boleh menyelesaikannya.
- **Budget** (`requires_budget`): `budget_amount` wajib > 0 sebelum step selesai.
- **Stage** work item otomatis maju ke phase langkah terbuka paling awal.

## Menambah / mengubah alur
Cukup edit `StepTemplate` di
[`backend/internal/service/catalog.go`](backend/internal/service/catalog.go) dan
(opsional) hint UI di `greenparkmarketing/src/lib/alurCatalog.ts`. Engine sudah
generik.

> Frontend workflow ada di `greenparkmarketing/` (React + TypeScript + Vite),
> juga mengikuti pola permit (`models / services / context / pages / components`).
