/** @type {import('next').NextConfig} */
const api = process.env.LINEJUDGE_API_URL || "http://127.0.0.1:8080";

const nextConfig = {
  output: "standalone",
  async rewrites() {
    return [
      { source: "/v1/:path*", destination: `${api}/v1/:path*` },
      { source: "/webhooks/:path*", destination: `${api}/webhooks/:path*` },
      { source: "/healthz", destination: `${api}/healthz` },
    ];
  },
};

module.exports = nextConfig;
