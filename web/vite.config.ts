import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'
import http from 'node:http'

// Override with VITE_API_TARGET env var for remote servers, e.g.:
//   VITE_API_TARGET=http://10.80.111.2:8067 npm run dev -- --host
const apiTarget = process.env.VITE_API_TARGET || 'http://localhost:8067'
const targetUrl = new URL(apiTarget)

function apiProxyPlugin(): Plugin {
  return {
    name: 'api-proxy',
    configureServer(server) {
      console.log(`\n  API proxy: /api/* → ${apiTarget}\n`)

      server.middlewares.use((req, res, next) => {
        const url = req.url || ''
        if (!url.startsWith('/api/') && !url.startsWith('/metrics')) {
          return next()
        }

        console.log(`  → proxy: ${req.method} ${url}`)

        const proxyReq = http.request(
          {
            hostname: targetUrl.hostname,
            port: targetUrl.port,
            path: url,
            method: req.method,
            headers: {
              ...req.headers,
              host: targetUrl.host,
            },
          },
          (proxyRes) => {
            res.writeHead(proxyRes.statusCode || 502, proxyRes.headers)
            proxyRes.pipe(res)
          },
        )

        proxyReq.on('error', (err) => {
          console.error(`  ✗ proxy error: ${url} → ${err.message}`)
          if (!res.headersSent) {
            res.writeHead(502, { 'Content-Type': 'application/json' })
          }
          res.end(JSON.stringify({ error: 'proxy_error', detail: err.message }))
        })

        req.pipe(proxyReq)
      })
    },
  }
}

export default defineConfig({
  plugins: [apiProxyPlugin(), react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
})
