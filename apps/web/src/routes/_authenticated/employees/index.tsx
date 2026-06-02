import { createFileRoute } from "@tanstack/react-router";
import { EmployeesPage } from "@/features/employees";

export const Route = createFileRoute("/_authenticated/employees/")({
  component: EmployeesPage,
});
