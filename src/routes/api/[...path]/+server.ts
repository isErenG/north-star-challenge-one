import { env } from '$env/dynamic/private';
import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
const proxy: RequestHandler = async ({ request, params }) => {
  if (!/^(config|jobs(?:\/[a-f0-9]{48})?)$/.test(params.path ?? ''))
    return json({ error: 'Not found' }, { status: 404 });
  if (request.method !== 'GET') {
    const origin = request.headers.get('origin');
    if (!origin || origin !== new URL(request.url).origin)
      return json({ error: 'Invalid request origin' }, { status: 403 });
    if (Number(request.headers.get('content-length') || 0) > 10 * 1024 * 1024)
      return json(
        { error: 'Upload a file smaller than 10 MB.' },
        { status: 413 }
      );
  }
  try {
    const response = await fetch(
      `${env.GO_API_URL || 'http://127.0.0.1:8787'}/api/${params.path}`,
      {
        method: request.method,
        headers: {
          'content-type':
            request.headers.get('content-type') || 'application/json'
        },
        body:
          request.method === 'GET' ? undefined : await request.arrayBuffer(),
        signal: AbortSignal.timeout(30000)
      }
    );
    return new Response(response.body, {
      status: response.status,
      headers: {
        'content-type': 'application/json',
        'cache-control': 'no-store'
      }
    });
  } catch {
    return json(
      {
        error:
          'The processing service is unavailable. Please try again shortly.'
      },
      { status: 503 }
    );
  }
};
export const GET = proxy;
export const POST = proxy;
export const PATCH = proxy;
