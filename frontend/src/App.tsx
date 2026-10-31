import { CreateOrderForm } from './components/CreateOrderForm.tsx'
import { DashboardStats } from './components/DashboardStats.tsx'
import { OrdersTable } from './components/OrdersTable.tsx'

export default function App() {
  return (
    <main className="dashboard">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight">Orders Dashboard</h1>
      </header>

      <DashboardStats />
      <OrdersTable />
      <CreateOrderForm />
    </main>
  )
}
