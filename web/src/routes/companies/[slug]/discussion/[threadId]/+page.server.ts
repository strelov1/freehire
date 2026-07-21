import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params, fetch }) => {
  const id = Number(params.threadId);
  if (!Number.isInteger(id)) error(404, 'Thread not found');
  const api = serverApi(fetch);
  try {
    const { thread, replies, nextCursor } = await api.getThread(id);
    return { slug: params.slug, thread, replies, nextCursor };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) error(404, 'Thread not found');
    throw e;
  }
};
