import { createFileRoute } from "@tanstack/react-router";
import { Users } from "@/features/users";

export type UsersSearch = {
  user?: string;
};

export const Route = createFileRoute("/_authenticated/users/")({
  component: UsersRoute,
  validateSearch: (search: Record<string, unknown>): UsersSearch => {
    const result: UsersSearch = {};
    if (typeof search.user === "string" && search.user.trim()) {
      result.user = search.user.trim();
    }
    return result;
  },
});

function UsersRoute() {
  const { user } = Route.useSearch();
  return <Users initialUserId={user} />;
}
