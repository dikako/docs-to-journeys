# Journey: Checkout one item

## Results

### Action: Tap "Hamburger" menu ✅
- **Commands**:
  - `adb shell input tap 71 197`
- **Comment**: Found as content-desc "View menu" (menuIV); the drawer opened.

### Action: Tap "Log In" menu ✅
- **Commands**:
  - `adb shell input tap 365 1735`
- **Comment**: Found as content-desc "Login Menu Item"; the Login screen opened.

### Action: Type "standard_user" in the Username field ✅
- **Commands**:
  - `adb shell input tap 540 821`
  - `adb shell input text standard_user`
- **Comment**: The field (nameET) had FOCUSED state before typing.

### Action: Type "secret_sauce" in the Password field ✅
- **Commands**:
  - `adb shell input tap 540 1087`
  - `adb shell input text secret_sauce`
- **Comment**: The field (passwordET) had FOCUSED state before typing.

### Action: Tap the "LOGIN" button ✅
- **Commands**:
  - `adb shell input tap 540 1416`
- **Comment**: The button text is "Login" (content-desc "Tap to login with given credentials").

### Action: Verify that the "PRODUCTS" screen is shown with at least one product listed ✅
- **Comment**: The title reads "Products" and the product list contains "Sauce Labs Backpack" and other items.

### Action: Tap the "Sauce Labs Backpack" product to open its details ✅
- **Commands**:
  - `adb shell input tap 231 1130`
  - `adb shell input tap 285 800`
- **Comment**: The title text is not clickable, so the first tap did nothing. Tapping the product image opened the details.

### Action: Tap "ADD TO CART" ✅
- **Commands**:
  - `adb shell input tap 710 2210`
- **Comment**: The button text is "Add to cart". The cart badge showed 1 afterwards.

### Action: Tap the cart icon at the top of the screen ✅
- **Commands**:
  - `adb shell input tap 1008 197`
- **Comment**: The My Cart screen opened with Sauce Labs Backpack.

### Action: Tap the "CHECKOUT" button. This should open the checkout information form ✅
- **Commands**:
  - `adb shell input tap 540 2169`
- **Comment**: The button text is "Proceed To Checkout", not "CHECKOUT". The checkout form opened.

### Action: Type "Dika" in the First Name field ❌
- **Comment**: The checkout form has no First Name field. Its fields are Full Name, Address Line 1, Address Line 2, City, Zip Code, State/Region and Country. The journey stops here.

### Action: Type "QA" in the Last Name field

### Action: Type "12345" in the Zip/Postal Code field

### Action: Tap the "CONTINUE" button. This should open the checkout overview

### Action: Verify that "Sauce Labs Backpack" and a total price are shown on the overview

### Action: Scroll down and tap the "FINISH" button

### Action: Verify that an order confirmation / thank-you message is shown
