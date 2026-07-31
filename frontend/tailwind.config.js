/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        dark: '#1A1D24',
        darkCard: '#232730',
        brand: '#dd763d',      // fox orange
        brandGreen: '#10B981',
        brandRed: '#EF4444',
      }
    },
  },
  plugins: [],
}
