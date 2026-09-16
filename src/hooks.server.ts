import { localeCookie, resolveLocale } from '$lib/i18n';
import type { Handle } from '@sveltejs/kit';

export const handle: Handle = ({ event, resolve }) => {
  const locale = resolveLocale(event.cookies.get(localeCookie));
  return resolve(event, {
    transformPageChunk: ({ html }) =>
      html.replace('lang="en"', `lang="${locale}"`)
  });
};
