"use client";

import { Copy, ExternalLink, FolderPlus } from "lucide-react";
import { Button, Modal, Space, Tag } from "antd";

import { formatPromptDate, type Prompt } from "@/services/api/prompts";

export function PromptDetailDialog({ prompt, onClose, onCopy, onSaveAsset }: { prompt: Prompt | null; onClose: () => void; onCopy: (prompt: string) => void; onSaveAsset?: (prompt: Prompt) => void }) {
    return (
        <>
            <Modal title={prompt?.title} open={Boolean(prompt)} onCancel={onClose} footer={null} width={860}>
                {prompt ? (
                    <>
                        <div className="grid gap-5 md:grid-cols-[300px_minmax(0,1fr)]">
                            <div className="space-y-3">
                                <img src={prompt.coverUrl} alt={prompt.title} className="aspect-[4/3] w-full rounded-lg object-cover" />
                                {prompt.preview ? <pre className="max-h-60 overflow-auto whitespace-pre-wrap rounded-lg bg-neutral-100 p-3 text-xs leading-5 text-neutral-600 dark:bg-neutral-900 dark:text-neutral-300">{prompt.preview}</pre> : null}
                            </div>
                            <div className="min-w-0">
                                <div className="flex flex-wrap gap-1.5">
                                    {prompt.tags.map((tag) => (
                                        <Tag key={tag} className="m-0">
                                            {tag}
                                        </Tag>
                                    ))}
                                </div>
                                <p className="mt-4 whitespace-pre-wrap text-sm leading-7 text-neutral-800 dark:text-neutral-300">{prompt.prompt}</p>
                                <div className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-neutral-500 dark:text-neutral-400">
                                    <span>
                                        创建：{formatPromptDate(prompt.createdAt)} · 更新：{formatPromptDate(prompt.updatedAt)}
                                    </span>
                                    {prompt.githubUrl ? (
                                        <a href={prompt.githubUrl} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary transition hover:underline">
                                            GitHub 来源
                                            <ExternalLink className="size-3" />
                                        </a>
                                    ) : null}
                                </div>
                                <Space wrap className="mt-5">
                                    <Button type="primary" icon={<Copy className="size-4" />} onClick={() => onCopy(prompt.prompt)}>
                                        复制提示词
                                    </Button>
                                    {onSaveAsset ? (
                                        <Button icon={<FolderPlus className="size-4" />} onClick={() => onSaveAsset(prompt)}>
                                            加入我的素材
                                        </Button>
                                    ) : null}
                                </Space>
                            </div>
                        </div>
                    </>
                ) : null}
            </Modal>
        </>
    );
}
