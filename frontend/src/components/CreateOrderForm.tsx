import { useState, type FormEvent } from 'react'
import { formatApiError } from '@/api/client'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { useCreateOrder } from '@/hooks/useOrders'
import { useProducts } from '@/hooks/useProducts'
import { formatPrice } from '@/lib/format'
import type { Product } from '@/types/product'

function validateQuantity(product: Product | undefined, quantity: string): string | null {
  if (!product) {
    return 'Select a product.'
  }

  if (product.stockQuantity < 1) {
    return 'This product is out of stock.'
  }

  if (!/^\d+$/.test(quantity.trim())) {
    return 'Quantity must be a whole number.'
  }

  const parsedQuantity = Number(quantity)
  if (parsedQuantity < 1 || parsedQuantity > product.stockQuantity) {
    return `Quantity must be between 1 and ${product.stockQuantity}.`
  }

  return null
}

export function CreateOrderForm() {
  const products = useProducts()
  const createOrder = useCreateOrder()
  const [productId, setProductId] = useState('')
  const [quantity, setQuantity] = useState('1')
  const [formKey, setFormKey] = useState(0)
  const [formError, setFormError] = useState<string | null>(null)

  const selectedProduct = products.data?.find((product) => product.id === productId)

  function clearFeedback() {
    setFormError(null)
    if (createOrder.isError) {
      createOrder.reset()
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const validationError = validateQuantity(selectedProduct, quantity)
    if (validationError) {
      setFormError(validationError)
      return
    }

    if (!selectedProduct) {
      return
    }

    setFormError(null)
    createOrder.mutate(
      {
        productId: selectedProduct.id,
        quantity: Number(quantity),
      },
      {
        onSuccess: () => {
          setProductId('')
          setQuantity('1')
          setFormError(null)
          setFormKey((key) => key + 1)
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <h2>Create New Order</h2>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {products.isError ? (
          <Alert variant="destructive" className="mb-4">
            <AlertTitle>Could not load products</AlertTitle>
            <AlertDescription>
              {formatApiError(products.error, 'The products service is unavailable.')}
            </AlertDescription>
          </Alert>
        ) : null}

        <form className="grid gap-4" noValidate onSubmit={handleSubmit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="product">Product</Label>
              {products.isPending ? (
                <Skeleton className="h-8 w-full" />
              ) : (
                <Select
                  key={formKey}
                  value={productId}
                  onValueChange={(value) => {
                    setProductId(value)
                    clearFeedback()
                  }}
                  disabled={!products.data?.length || createOrder.isPending}
                >
                  <SelectTrigger id="product" className="w-full min-w-0">
                    <SelectValue placeholder="Select product" />
                  </SelectTrigger>
                  <SelectContent position="popper">
                    {products.data?.map((product) => (
                      <SelectItem key={product.id} value={product.id}>
                        {product.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              {selectedProduct ? (
                <p className="text-sm text-muted-foreground">
                  Price: {formatPrice(selectedProduct.priceCents)} · In stock:{' '}
                  {selectedProduct.stockQuantity}
                </p>
              ) : null}
              {products.data && products.data.length === 0 ? (
                <p className="text-sm text-muted-foreground">No products available.</p>
              ) : null}
            </div>

            <div className="grid gap-2">
              <Label htmlFor="quantity">Quantity</Label>
              <Input
                id="quantity"
                type="number"
                min={1}
                max={selectedProduct?.stockQuantity}
                step={1}
                value={quantity}
                aria-invalid={Boolean(formError)}
                disabled={createOrder.isPending}
                onChange={(event) => {
                  setQuantity(event.target.value)
                  clearFeedback()
                }}
              />
            </div>
          </div>

          {formError ? (
            <Alert variant="destructive">
              <AlertTitle>Check the order</AlertTitle>
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}

          {createOrder.isError ? (
            <Alert variant="destructive">
              <AlertTitle>Could not create order</AlertTitle>
              <AlertDescription>
                {formatApiError(
                  createOrder.error,
                  'The order service is unavailable.',
                )}
              </AlertDescription>
            </Alert>
          ) : null}

          <div>
            <Button
              type="submit"
              disabled={
                createOrder.isPending || products.isPending || !products.data?.length
              }
            >
              {createOrder.isPending ? 'Creating...' : 'Create Order'}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
