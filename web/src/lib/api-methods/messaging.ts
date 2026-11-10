import type { Http } from '../api-core';
import type { Conversation, ConversationMessage, ConversationReport } from '../types';
import { jsonBody } from '../api-core';

export function messagingApi({ call, requestData }: Http) {
  // ── Conversations (the messaging core, every kind) ───────────────────────
  async function listConversations(): Promise<Conversation[]> {
    return requestData('/api/v1/me/conversations');
  }

  async function answerConversation(id: number, action: 'accept' | 'decline'): Promise<Conversation> {
    return requestData(`/api/v1/me/conversations/${id}/${action}`, jsonBody('POST', {}));
  }

  async function blockConversation(id: number): Promise<void> {
    await call(`/api/v1/me/conversations/${id}/block`, { method: 'POST' });
  }

  async function reportConversation(id: number, reason: ConversationReport['reason'], note: string): Promise<void> {
    await call(`/api/v1/me/conversations/${id}/report`, jsonBody('POST', { reason, note }));
  }

  async function listConversationMessages(id: number, after?: number, limit?: number): Promise<ConversationMessage[]> {
    const params = new URLSearchParams();
    if (after) params.set('after', String(after));
    if (limit) params.set('limit', String(limit));
    const q = params.size ? `?${params}` : '';
    return requestData(`/api/v1/me/conversations/${id}/messages${q}`);
  }

  async function sendConversationMessage(id: number, body: string): Promise<ConversationMessage> {
    return requestData(`/api/v1/me/conversations/${id}/messages`, jsonBody('POST', { body }));
  }

  async function markConversationRead(id: number, upTo: number): Promise<void> {
    await call(`/api/v1/me/conversations/${id}/read`, jsonBody('POST', { up_to: upTo }));
  }

  async function listConversationReports(
    limit = 30,
    offset = 0,
    includeResolved = false,
  ): Promise<ConversationReport[]> {
    const q = new URLSearchParams({
      limit: String(limit),
      offset: String(offset),
      include_resolved: String(includeResolved),
    });
    return requestData(`/api/v1/moderation/conversation-reports?${q}`);
  }

  async function resolveConversationReport(id: number): Promise<void> {
    await call(`/api/v1/moderation/conversation-reports/${id}/resolve`, jsonBody('POST', {}));
  }

  return {
    listConversations,
    answerConversation,
    blockConversation,
    reportConversation,
    listConversationMessages,
    sendConversationMessage,
    markConversationRead,
    listConversationReports,
    resolveConversationReport,
  };
}
