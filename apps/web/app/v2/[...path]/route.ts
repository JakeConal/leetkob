import { proxyV2Request } from "../../../lib/proxy";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

interface RouteContext {
  readonly params: Promise<{ readonly path: readonly string[] }>;
}

export async function GET(request: Request, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxyV2Request("GET", request, path);
}

export async function POST(request: Request, context: RouteContext): Promise<Response> {
  const { path } = await context.params;
  return proxyV2Request("POST", request, path);
}
