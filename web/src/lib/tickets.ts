import type { TicketsView } from "./api";

/** titleOf is a ticket's title, open or done, as GitHub last said it. */
export function titleOf(tickets: TicketsView | undefined, issue: number): string | undefined {
  const open = tickets?.tickets?.open?.find((t) => t.number === issue);
  return open?.title ?? tickets?.tickets?.done_titles?.[String(issue)];
}
