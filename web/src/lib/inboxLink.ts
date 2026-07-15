/** The link control the inbox reading pane shows for the selected email. */
export type InboxLinkState = 'linked' | 'suggested' | 'undo' | 'picker';

/** The subset of an email the link-state decision reads. */
export interface LinkStateEmail {
	id: number;
	linked_slug?: string;
	suggested_slug?: string;
}

/** The application an email was just unlinked from, remembered for Undo. */
export interface LastUnlinked {
	id: number;
	slug: string;
	company?: string;
}

/** Decide which link control to render: a real link wins, then a pending
 *  suggestion, then a matching just-unlinked stash offers Undo, else the picker. */
export function inboxLinkState(email: LinkStateEmail, lastUnlinked: LastUnlinked | null): InboxLinkState {
	if (email.linked_slug) return 'linked';
	if (email.suggested_slug) return 'suggested';
	if (lastUnlinked && lastUnlinked.id === email.id) return 'undo';
	return 'picker';
}
