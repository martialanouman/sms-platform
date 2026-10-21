import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { newRouter } from './router'
import './styles/app.css'
import { retry } from './session'

const root = document.getElementById('root')
if (root) {
  createRoot(root).render(
    <StrictMode>
      <QueryClientProvider
        client={
          new QueryClient({
            defaultOptions: {
              queries: { retry },
            },
          })
        }
      >
        <RouterProvider router={newRouter()} />
      </QueryClientProvider>
    </StrictMode>,
  )
}
