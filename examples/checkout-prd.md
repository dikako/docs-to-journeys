# PRD: Checkout (Sauce Labs My Demo App)

| | |
|---|---|
| App | Sauce Labs My Demo App (Android), package `com.saucelabs.mydemoapp.android` |
| Status | Example: used to demo `docs-to-journeys` |
| Owner | QA |

## Goal
A logged-in shopper can buy a product: add it to the cart, enter shipping and payment details, review the order and place it.

## Users and preconditions
- The shopper opens the side menu ("View menu"), taps "Log In", and logs in with username `bob@example.com` and password `10203040`.
- After login the shopper lands on the **Products** screen, which lists at least one product.

## Flow
1. On **Products**, the shopper opens "Sauce Labs Backpack" and taps **Add to cart**. The cart badge shows `1`.
2. The shopper taps the cart icon ("View cart"). **My Cart** lists "Sauce Labs Backpack".
3. The shopper taps **Proceed To Checkout**, which opens the shipping address form.
4. Shipping form fields: Full Name\*, Address Line 1\*, Address Line 2, City\*, State/Region, Zip Code\*, Country\* (\* = required). The shopper taps **To Payment**.
5. Payment form fields: Full Name\*, Card Number\*, Expiration Date\* (MM/YY), Security Code\*. The shopper taps **Review Order**.
6. **Review** shows the product, the shipping and payment details, and the total price. The shopper taps **Place Order**.
7. **Checkout Complete** shows "Thank you for your order".

## Test data
| Field | Value |
|---|---|
| Full Name | Dika QA |
| Address Line 1 | Jalan Sudirman 1 |
| City | Jakarta |
| Zip Code | 12190 |
| Country | Indonesia |
| Card Number | 4111111111111111 |
| Expiration Date | 03/28 |
| Security Code | 123 |

## Acceptance criteria
1. **Happy path:** with the test data above, the shopper reaches **Checkout Complete** and sees "Thank you for your order".
2. **Review shows the order:** the review screen shows "Sauce Labs Backpack" and a total price before the order is placed.
3. **Required shipping field:** if Full Name is left empty and the shopper taps **To Payment**, an error is shown under Full Name and the shopper stays on the shipping form.
4. **Cart badge:** after adding the backpack, the cart badge shows `1`.

## Out of scope
- Real payment processing, promo codes, multiple products.
