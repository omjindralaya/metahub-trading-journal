import {defineConfig, Plugin} from 'vite'
import react from '@vitejs/plugin-react'
import {writeFileSync} from 'node:fs'
import {resolve} from 'node:path'

// main.go melakukan `//go:embed all:frontend/dist`, dan embed GAGAL bila polanya
// tidak cocok dengan apa pun. Karena itu direktori dist harus tetap ada di git
// (lewat .gitkeep) supaya `go build` jalan pada clone segar.
//
// Masalahnya, build Vite mengosongkan outDir lebih dulu (emptyOutDir default
// true) dan ikut membuang .gitkeep — sehingga setiap kali seseorang membangun
// frontend, git melaporkan file itu terhapus. Menulisnya kembali setelah bundle
// selesai lebih baik daripada mematikan emptyOutDir, yang akan membuat aset
// ber-hash lama menumpuk di dalam binary.
function keepDistTracked(): Plugin {
  return {
    name: 'keep-dist-tracked',
    apply: 'build',
    closeBundle() {
      writeFileSync(
        resolve(__dirname, 'dist/.gitkeep'),
        'Placeholder agar direktori ini tetap ada di git; lihat catatan di vite.config.ts.\n',
      )
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), keepDistTracked()],
  build: {
    rollupOptions: {
      output: {
        // Recharts sendirian mendominasi bundle (411 kB dari 668 kB). Digabung
        // dengan kode aplikasi, setiap perubahan kecil pada UI menghasilkan ulang
        // satu chunk raksasa. Dipisah, chunk itu stabil antar-rilis.
        //
        // React TIDAK dipisah: react-dom sudah menariknya, jadi entri terpisah
        // hanya menghasilkan chunk kosong 0 kB dan peringatan dari rollup.
        manualChunks: {
          charts: ['recharts'],
        },
      },
    },
  },
})
