# Plan: Perbaikan Error Handling Frontend (`[object Object]`)

**Tanggal:** 2026-07-09
**Status:** SELESAI (2026-07-09)
**Penyebab:** Backend `metahub-api` mengganti bentuk field `error` dari **string** → **object** (breaking change yang disengaja, lihat memori `metahub-response-contract`). Frontend masih merender `error` mentah, sehingga tampil `[object Object]`.

---

## 1. Apa yang berubah di backend

Envelope respons sekarang (single source of truth: `metahub-api/pkg/response`):

```jsonc
// SUKSES
{ "success": true, "message": "...", "data": {...}, "request_id": "..." }

// ERROR  (error = OBJECT, bukan string lagi)
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",        // slug stabil untuk branching
    "message": "Request validation failed", // teks untuk ditampilkan
    "details": [                        // opsional, error per-field
      { "field": "email", "issue": "must be a valid email" }
    ]
  },
  "request_id": "a846..."
}
```

Kode error yang mungkin muncul: `VALIDATION_ERROR`, `UNAUTHORIZED`, `FORBIDDEN`,
`NOT_FOUND`, `BAD_REQUEST`, `CONFLICT`, `INTERNAL_ERROR`.

**Aturan tampil:** gunakan `error.message` untuk teks; branch pada `error.code`;
map `error.details[]` ke error per-field di form (opsional).

---

## 2. Kenapa muncul `[object Object]`

Pola lama di frontend kira-kira:

```ts
} catch (err: any) {
    const errMsg = typeof err === 'string' ? err : err?.message || String(err);
    // err di sini adalah body { success, error: {code, message}, ... }
    // -> err.message = undefined
    // -> String(err) = "[object Object]"
    setError(errMsg); // atau toast.error(errMsg)
}
```

`err.message` tidak ada karena pesan sekarang ada di `err.error.message`.

---

## 3. Lokasi yang perlu diubah

> Aplikasi ini **Wails desktop**: panggilan HTTP ke backend terjadi di layer Go
> (`wailsjs/go/main/App` → `LoginToCloud`, dll), lalu error dilempar ke JS.
> Ada DUA tempat yang menentukan bentuk error yang diterima JS:

### a. Layer Go (Wails) — cek dulu, ini kemungkinan sumber utama
Cari fungsi `LoginToCloud` (dan sync/register lain) di kode Go Wails
(`trading-journal/frontend/` induk, atau root app Go). Pastikan saat HTTP status
bukan 2xx, ia mengekstrak `body.error.message` lalu `return errors.New(msg)` —
**bukan** mengembalikan seluruh body atau `[object Object]`.

Contoh perbaikan di Go:
```go
if resp.StatusCode >= 400 {
    var env struct {
        Error struct {
            Code    string `json:"code"`
            Message string `json:"message"`
        } `json:"error"`
    }
    _ = json.NewDecoder(resp.Body).Decode(&env)
    if env.Error.Message != "" {
        return fmt.Errorf("%s", env.Error.Message) // -> JS dapat string bersih
    }
    return fmt.Errorf("request failed (%d)", resp.StatusCode)
}
```

### b. Komponen login "Welcome back" (React)
File ini BELUM terkonfirmasi (bukan `LoginCloudModal.tsx`). Temukan dengan:
```
grep -rn "following feed\|Create an account\|Welcome back" trading-journal/frontend/src
```
Kalau tidak ada di `frontend/src`, berarti halaman itu bagian dari frontend/web
lain — cari di seluruh repo. Di komponen itu, perbaiki blok `catch`:

```ts
// SEBELUM
} catch (err: any) {
    setError(err?.message || String(err)); // -> [object Object]
}

// SESUDAH
} catch (err: any) {
    setError(extractApiError(err));
}
```

---

## 4. Helper terpusat (disarankan)

Buat `src/lib/apiError.ts`:

```ts
// Menerima: Error string dari Wails, ATAU body envelope { error: {message} }.
export function extractApiError(err: unknown): string {
    if (typeof err === 'string') return err;
    if (err && typeof err === 'object') {
        const anyErr = err as any;
        // envelope backend
        if (anyErr.error?.message) return anyErr.error.message as string;
        // Error biasa
        if (typeof anyErr.message === 'string') return anyErr.message;
    }
    return 'Terjadi kesalahan. Coba lagi.';
}

// Opsional: error per-field untuk form
export function fieldErrors(err: unknown): Record<string, string> {
    const details = (err as any)?.error?.details;
    if (!Array.isArray(details)) return {};
    return Object.fromEntries(details.map((d: any) => [d.field, d.issue]));
}
```

Ganti semua pola `err?.message || String(err)` (di `LoginCloudModal.tsx:44`,
`Header.tsx:47`, `Sidebar.tsx:57`, dan komponen login "Welcome back") dengan
`extractApiError(err)`.

---

## 5. Checklist

- [x] Cek layer Go Wails: SUDAH beres di `backend/sync_service.go` — ada struct
      `CloudError{code,message}` + `(*CloudError).Text()`, dipakai di `LoginToCloud`,
      `SyncToCloud`, dan sync posisi terbuka. Wails mengirim ini ke JS sebagai
      **string bersih**, jadi akar `[object Object]` sudah hilang di layer Go.
- [x] Komponen login "Welcome back" TIDAK ADA di `frontend/src` (item spekulatif,
      dilewati). Semua panggilan backend lewat binding Wails Go, tidak ada `fetch()`
      langsung ke `:8080` di frontend.
- [x] Tambah `src/lib/apiError.ts` (`extractApiError`, `apiErrorCode`, `fieldErrors`)
- [x] Ganti pola error → `extractApiError(err)` di:
      `LoginCloudModal.tsx`, `Header.tsx` (+ deteksi sesi via `apiErrorCode==="UNAUTHORIZED"`),
      `Sidebar.tsx`
- [x] `npx tsc --noEmit` lolos tanpa error
- [ ] (Opsional) Tampilkan `fieldErrors(err)` per-input di form login/register
- [ ] Uji manual di app: login kredensial salah → pesan bersih (bukan `[object Object]`)
```
```

---

## Catatan verifikasi cepat (tanpa frontend)

Bentuk error backend bisa dicek langsung:
```
curl -s http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" -d '{"email":"x"}'
# -> {"success":false,"error":{"code":"VALIDATION_ERROR","message":"...","details":[...]},"request_id":"..."}
```
