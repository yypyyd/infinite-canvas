import { apiDelete, apiGet, apiPost, compactApiParams, type ApiParams } from "@/services/api/request";
import type { Product, ProductSKU } from "@/services/api/commerce";

export const fetchCommerceProducts = (params?: ApiParams) => apiGet<{ items: Product[]; total: number }>("/api/commerce/products", compactApiParams(params || {}));
export const fetchCommerceProduct = async (id: string) => {
    const pageSize = 100;
    let page = 1;
    while (true) {
        const result = await fetchCommerceProducts({ page, pageSize });
        const product = result.items.find((item) => item.id === id);
        if (product) return product;
        if (page * pageSize >= result.total || result.items.length === 0) throw new Error("商品不存在");
        page += 1;
    }
};
export const fetchCommerceProductSKUs = (productId: string, params?: ApiParams) => apiGet<{ items: ProductSKU[]; total: number }>(`/api/commerce/products/${productId}/skus`, compactApiParams(params || {}));
export const saveCommerceProduct = (input: Partial<Product>) => apiPost<Product>("/api/commerce/products", input);
export const deleteCommerceProduct = (id: string, expectedVersion: number) => apiDelete<boolean>(`/api/commerce/products/${id}?expectedVersion=${expectedVersion}`);
export const saveCommerceProductSKU = (input: Partial<ProductSKU>) => apiPost<ProductSKU>("/api/commerce/skus", input);
export const deleteCommerceProductSKU = (id: string, expectedVersion: number) => apiDelete<boolean>(`/api/commerce/skus/${id}?expectedVersion=${expectedVersion}`);
