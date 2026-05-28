import {
  fetchGroups,
  createGroup as apiCreate,
  updateGroup as apiUpdate,
  deleteGroup as apiDelete
} from '$lib/api'
import type { Group } from '$lib/api'

// groupsStore is the single source of truth for camera-group definitions.
// Mutations always hit the BFF then refresh the local snapshot, so the
// single-membership invariant remains visible across cards.
class GroupsStore {
  groups = $state<Group[]>([])
  loaded = $state(false)
  #initStarted = false

  async init() {
    if (this.#initStarted) return
    this.#initStarted = true
    await this.refresh()
  }

  async refresh() {
    try {
      this.groups = await fetchGroups()
    } catch {
      // Keep last-known state on transient errors; UI surfaces stale data
      // rather than blanking out.
    } finally {
      this.loaded = true
    }
  }

  async create(name: string): Promise<Group> {
    const created = await apiCreate(name)
    await this.refresh()
    return created
  }

  async update(id: string, patch: { name?: string; camera_ids?: string[] }): Promise<Group> {
    const updated = await apiUpdate(id, patch)
    await this.refresh()
    return updated
  }

  async remove(id: string): Promise<void> {
    await apiDelete(id)
    await this.refresh()
  }

  byId(id: string): Group | undefined {
    return this.groups.find((g) => g.id === id)
  }
}

export const groupsStore = new GroupsStore()
