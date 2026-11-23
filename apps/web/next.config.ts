import type { NextConfig } from "next";

const pa3ApiOrigin = process.env["PA3_API_ORIGIN"] ?? "http://127.0.0.1:8080";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/v2/:path*",
        destination: `${pa3ApiOrigin}/v2/:path*`,
      },
    ];
  },
};

export default nextConfig;
