import { createFileRoute } from "@tanstack/react-router";
import { CreateEmployeePage } from "@/features/employees/create";

export const Route = createFileRoute("/_authenticated/employees/new")({
  component: CreateEmployeePage,
});
