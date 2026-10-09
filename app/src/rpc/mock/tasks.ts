// The tasks domain of MockTransport: the fixture's task lists and tasks, as
// maild answers tasks.* (docs/design/pim.md, Tasks; internal/engine/tasks.go).
// Task T-0084 builds it.
import type { Collection, Event, Task } from "../gen/api";
import { NOT_HANDLED } from "./compose";

/** MockTasksData is the tasks the mock serves, each list's in its order
 *  (a parent before its subtasks); their lists are `tasklist` collections
 *  in MockPeopleData. */
export interface MockTasksData {
  tasks: Task[];
  /** The time a task completed here is stamped with. */
  now: string;
}

/** MockTasks answers tasks.list, create, update and delete over the shared
 *  collections. */
export class MockTasks {
  constructor(
    readonly data: MockTasksData,
    readonly collections: Collection[],
    readonly emit: (e: Event) => void,
  ) {}

  /** dispatch answers a tasks method, or returns NOT_HANDLED. */
  dispatch(_method: string, _params: Record<string, unknown>): unknown {
    return NOT_HANDLED;
  }
}
