import { useProject } from "@/app/project";
import type { RoleView } from "@/lib/council";
import { cn } from "@/lib/utils";

/**
 * Each role of the council has a line colour of its own, as each line of a
 * transit map has: the colour says who speaks before the name is read. None
 * of them is the signal's yellow, which is kept for what needs the owner.
 */
export const LINE = ["#0E7C86", "#B4235A", "#2F6FDE", "#7A5AF8", "#C2410C", "#15803D", "#8A5A00", "#475569"];

export interface Seat {
  id: string;
  title: string;
  color: string;
  role?: RoleView;
}

const MODERATOR: Seat = { id: "moderator", title: "主持人", color: "#14202C" };

/** useSeats tells the seats of the table, each with its colour, by role id. */
export function useSeats(): (id?: string) => Seat {
  const { roster } = useProject();
  const roles = roster.data?.roles ?? [];
  return (id?: string) => {
    if (!id || id === "moderator") return MODERATOR;
    const at = roles.findIndex((r) => r.id === id);
    if (at >= 0) return { id, title: roles[at].title, color: LINE[at % LINE.length], role: roles[at] };
    let hash = 0;
    for (const c of id) hash = (hash * 31 + c.charCodeAt(0)) >>> 0;
    return { id, title: id, color: LINE[hash % LINE.length] };
  };
}

/** initials are what a role's badge shows: the first letters of its title. */
function initials(seat: Seat): string {
  if (seat.id === "moderator") return "主";
  const words = seat.title.split(/[\s-]+/).filter(Boolean);
  const letters = words.length > 1 ? words[0][0] + words[1][0] : seat.title.slice(0, 2);
  return letters.toUpperCase();
}

export function RoleBadge({ seat, size = 32, ring }: { seat: Seat; size?: number; ring?: boolean }) {
  return (
    <span
      title={seat.title}
      aria-hidden
      className={cn("grid shrink-0 place-items-center rounded-[9px] font-black text-white", ring && "ring-[3px] ring-offset-2 ring-offset-card")}
      style={{ width: size, height: size, background: seat.color, fontSize: Math.round(size * 0.38), ["--tw-ring-color" as string]: seat.color }}
    >
      {initials(seat)}
    </span>
  );
}

/** RoleName is a role's badge and title, as a line of what was said starts. */
export function RoleName({ seat, size = 28, note }: { seat: Seat; size?: number; note?: React.ReactNode }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <RoleBadge seat={seat} size={size} />
      <span className="truncate text-[14px] font-extrabold">{seat.title}</span>
      {note}
    </span>
  );
}
