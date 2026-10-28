import { createRouter, createWebHistory } from 'vue-router'
import InvoiceDetailView from './views/InvoiceDetailView.vue'
import InvoicesView from './views/InvoicesView.vue'
import PaymentsView from './views/PaymentsView.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: InvoicesView },
    { path: '/invoices/:id', component: InvoiceDetailView },
    { path: '/payments', component: PaymentsView },
    { path: '/about', component: () => import('./views/AboutView.vue') },
  ],
})
