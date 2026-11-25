"use client";

import { ReloadOutlined } from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Card, Empty, Flex, Image, Input, Pagination, Spin, Typography } from "antd";
import dayjs from "dayjs";
import { useState } from "react";

import { adminGeneratedImageUrl, fetchAdminGeneratedImages } from "@/services/api/admin";
import { useUserStore } from "@/stores/use-user-store";

export function GeneratedImages() {
    const token = useUserStore((state) => state.token);
    const [keyword, setKeyword] = useState("");
    const [page, setPage] = useState(1);
    const pageSize = 24;
    const query = useQuery({
        queryKey: ["admin-generated-images", token, keyword, page],
        queryFn: () => fetchAdminGeneratedImages(token, { keyword, page, pageSize }),
        enabled: Boolean(token),
    });
    const images = query.data?.items ?? [];

    return (
        <Card variant="borderless">
            <Flex vertical gap={20}>
                <Flex gap={8} wrap>
                    <Input.Search
                        allowClear
                        placeholder="搜索用户 ID 或用户名"
                        style={{ maxWidth: 320 }}
                        onSearch={(value) => {
                            setKeyword(value.trim());
                            setPage(1);
                        }}
                    />
                    <Button icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => void query.refetch()}>
                        刷新
                    </Button>
                </Flex>
                {query.isError ? (
                    <Alert type="error" showIcon message="图片列表加载失败" />
                ) : query.isLoading ? (
                    <Spin />
                ) : images.length === 0 ? (
                    <Empty description="暂无生成图片" />
                ) : (
                    <div className="grid gap-4" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))" }}>
                        {images.map((item) => {
                            const url = adminGeneratedImageUrl(item.id);
                            return (
                                <figure key={item.id} className="m-0 min-w-0">
                                    <Image src={`${url}?variant=thumb`} preview={{ src: url }} alt={`${item.username || "用户"}的生成图片`} width="100%" loading="lazy" style={{ aspectRatio: "4 / 3", objectFit: "contain" }} />
                                    <figcaption className="mt-2 flex flex-col">
                                        <Typography.Text ellipsis>
                                            {item.username || "已删除用户"}
                                            {item.publicUserId != null && ` · ID ${item.publicUserId}`}
                                        </Typography.Text>
                                        <Typography.Text type="secondary">{dayjs(item.createdAt).format("YYYY-MM-DD HH:mm")}</Typography.Text>
                                    </figcaption>
                                </figure>
                            );
                        })}
                    </div>
                )}
                <Pagination current={page} pageSize={pageSize} total={query.data?.total ?? 0} onChange={setPage} showSizeChanger={false} hideOnSinglePage />
            </Flex>
        </Card>
    );
}
