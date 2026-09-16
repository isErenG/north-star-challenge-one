import { localeCookie, resolveLocale } from '$lib/i18n';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = ({ cookies }) => ({
  locale: resolveLocale(cookies.get(localeCookie))
});
