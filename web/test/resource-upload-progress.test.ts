import { expect, test } from "bun:test";
import type { AxiosProgressEvent } from "axios";
import { apiClient, directUploadClient } from "../src/services/api/request";
import { uploadResourceFile } from "../src/services/api/resources";

// 单流直传（≤5MB）走 /resources/direct-upload/init 拿预签名 URL，字节由 directUploadClient
// 直发对象存储；两个客户端都要打桩，否则 init 拿不到 session 会静默回退到分片链路。
function stubDirectUploadSession() {
    apiClient.defaults.adapter = async (config) => {
        const isInit = config.url === "/resources/direct-upload/init";
        const data = isInit
            ? {
                  session: {
                      resource: { id: "upload-test" },
                      uploadUrl: "https://oss.test/object",
                      method: "PUT",
                      headers: {},
                      expiresAt: "",
                  },
              }
            : { resource: { id: "upload-test" } };
        return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
    };
}

test("普通 multipart 将传输进度换算为文件字节，总量未知不伪造百分比", async () => {
    const apiAdapter = apiClient.defaults.adapter;
    const directAdapter = directUploadClient.defaults.adapter;
    const progress: number[][] = [];
    try {
        stubDirectUploadSession();
        directUploadClient.defaults.adapter = async (config) => {
            config.onUploadProgress?.({ loaded: 100, total: undefined } as AxiosProgressEvent);
            config.onUploadProgress?.({ loaded: 250, total: 1000 } as AxiosProgressEvent);
            config.onUploadProgress?.({ loaded: 1000, total: 1000 } as AxiosProgressEvent);
            return { config, status: 200, statusText: "OK", headers: {}, data: {} };
        };
        await uploadResourceFile(new Blob([new Uint8Array(400)]), "file", undefined, (loaded, total) => progress.push([loaded, total]));
        // 总量未知的那一次不上报，避免伪造百分比；后两次按 文件体积 × loaded/total 换算。
        expect(progress).toEqual([
            [100, 400],
            [400, 400],
        ]);
    } finally {
        apiClient.defaults.adapter = apiAdapter;
        directUploadClient.defaults.adapter = directAdapter;
    }
});

test("分片上传按已完成分片累加字节，合并响应返回前不发 100%", async () => {
    const adapter = apiClient.defaults.adapter;
    const size = 3 * 1024 * 1024;
    const chunkSize = 2 * 1024 * 1024;
    const progress: number[][] = [];
    let merged = false;
    try {
        apiClient.defaults.adapter = async (config) => {
            let data: unknown;
            if (config.url === "/resources/uploads") {
                data = { uploadId: "session", chunkSize, chunkCount: 2 };
            } else if (config.url?.endsWith("/complete")) {
                expect(progress.at(-1)).toEqual([size, size]);
                merged = true;
                data = { resource: { id: "chunk-test" } };
            } else {
                data = { index: Number(config.url?.split("/").at(-1)) };
            }
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 0, data } };
        };
        await uploadResourceFile(new Blob([new Uint8Array(size)]), "video", undefined, (loaded, total) => progress.push([loaded, total]));
        // 每片完成才累加一次：首片 2MB、次片 1MB，最终正好等于文件体积。
        expect(progress).toEqual([
            [chunkSize, size],
            [size, size],
        ]);
        expect(merged).toBe(true);
    } finally {
        apiClient.defaults.adapter = adapter;
    }
});

test("传输结束不吞掉服务端保存错误", async () => {
    const adapter = apiClient.defaults.adapter;
    try {
        apiClient.defaults.adapter = async (config) => {
            config.onUploadProgress?.({ loaded: 100, total: 100 } as AxiosProgressEvent);
            return { config, status: 200, statusText: "OK", headers: {}, data: { code: 9, msg: "保存失败" } };
        };
        await expect(uploadResourceFile(new Blob(["text"]), "file", undefined, () => {})).rejects.toThrow("保存失败");
    } finally {
        apiClient.defaults.adapter = adapter;
    }
});
