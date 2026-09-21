/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: {
          950: "#0b0c0f",
          900: "#111319",
          800: "#181b22",
          700: "#232833",
          600: "#2e3442",
        },
        brass: {
          300: "#f0c987",
          400: "#e4b45a",
          500: "#c9922e",
        },
      },
      fontFamily: {
        serif: ["Fraunces", "Georgia", "serif"],
        sans: ["IBM Plex Sans", "ui-sans-serif", "system-ui"],
        mono: ["IBM Plex Mono", "ui-monospace", "Menlo", "monospace"],
      },
    },
  },
  plugins: [],
};
