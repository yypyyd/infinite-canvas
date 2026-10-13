import { afterAll, beforeAll, expect, test } from "bun:test";

import { clearWorkspaceProjectConflict, commitWorkspaceChanges, readPendingWorkspaceChanges, rebasePendingWorkspaceChanges, setWorkspaceActorId, stageWorkspaceDelete, stageWorkspaceRecord } from "./workspace-changes.ts";

const previousWindow = globalThis.window;
beforeAll(() => {
    globalThis.window = new EventTarget();
});
afterAll(() => {
    globalThis.window = previousWindow;
    setWorkspaceActorId("");
});

test("冲突保存保留远端内容，后续本地修改复用同一个副本", () => {
    setWorkspaceActorId("actor-a");
    const owner = "actor-a:org-a";
    const remote = { domain: "canvas_project", objectId: "canvas-a", version: 2, deleted: false, updatedAt: "remote", data: { id: "canvas-a", title: "商品图", nodes: [{ id: "remote-node" }] } };
    stageWorkspaceRecord(owner, "canvas_project", "canvas-a", { id: "canvas-a", title: "商品图", nodes: [{ id: "local-node" }] }, 1);
    const { conflictCopies } = rebasePendingWorkspaceChanges(owner, [remote]);
    expect(conflictCopies).toHaveLength(1);
    const [copy] = readPendingWorkspaceChanges(owner);
    expect(copy.objectId).not.toBe("canvas-a");
    expect(copy.version).toBe(0);
    expect(copy.data.nodes).toEqual([{ id: "local-node" }]);
    expect(remote.data.nodes).toEqual([{ id: "remote-node" }]);
    const savedCopy = { ...copy, version: 1, updatedAt: "saved" };
    commitWorkspaceChanges([copy], [savedCopy]);

    stageWorkspaceRecord(owner, "canvas_project", "canvas-a", { id: "canvas-a", title: "商品图", nodes: [{ id: "later-edit" }] }, 2);
    expect(readPendingWorkspaceChanges(owner)[0].version).toBe(1);
    rebasePendingWorkspaceChanges(owner, [remote, savedCopy]);
    const [laterCopy] = readPendingWorkspaceChanges(owner);
    expect(laterCopy.objectId).toBe(copy.objectId);
    expect(laterCopy.version).toBe(1);
    expect(laterCopy.data.nodes).toEqual([{ id: "later-edit" }]);
    commitWorkspaceChanges([laterCopy], [{ ...laterCopy, version: 2 }]);

    stageWorkspaceRecord(owner, "canvas_project", "canvas-a", { id: "canvas-a", title: "商品图", nodes: [{ id: "third-edit" }] }, 2);
    rebasePendingWorkspaceChanges(owner, [remote, { ...savedCopy, version: 3 }]);
    const [newCopy] = readPendingWorkspaceChanges(owner);
    expect(newCopy.objectId).not.toBe(copy.objectId);
    expect(newCopy.version).toBe(0);
    commitWorkspaceChanges([newCopy], [{ ...newCopy, version: 1 }]);

    clearWorkspaceProjectConflict(owner, "canvas-a");
    stageWorkspaceRecord(owner, "canvas_project", "canvas-a", remote.data, 2);
    expect(readPendingWorkspaceChanges(owner)[0].version).toBe(2);
    commitWorkspaceChanges(readPendingWorkspaceChanges(owner), [remote]);
});

test("远端删除仍保留本地编辑副本，过期删除不会覆盖他人修改", () => {
    setWorkspaceActorId("actor-b");
    const owner = "actor-b:org-b";
    stageWorkspaceRecord(owner, "canvas_project", "removed", { id: "removed", title: "本地草稿", nodes: [{ id: "draft" }] }, 1);
    stageWorkspaceDelete(owner, "canvas_project", "keep", 1);
    const result = rebasePendingWorkspaceChanges(owner, [
        { domain: "canvas_project", objectId: "removed", version: 2, deleted: true },
        { domain: "canvas_project", objectId: "keep", version: 2, deleted: false },
    ]);
    expect(result.conflictCopies).toHaveLength(1);
    expect(result.skippedDeletes).toBe(1);
    expect(readPendingWorkspaceChanges(owner)).toHaveLength(1);
    const [copy] = readPendingWorkspaceChanges(owner);
    expect(copy.data.nodes).toEqual([{ id: "draft" }]);
    commitWorkspaceChanges([copy], [{ ...copy, version: 1 }]);
    clearWorkspaceProjectConflict(owner, "removed");
});
