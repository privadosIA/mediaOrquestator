# api-listing Specification

## Purpose
The parameters every list of the API accepts and what it answers with them, so that a client pages, sorts and searches every entity the same way, without changing what the lists answer today.

## Requirements

### Requirement: A list answers as before without parameters

A list of the API that existed before this capability SHALL answer, when it is asked with none of `limit`, `offset`, `sort` and `q`, the same body it answered before: the same shape, the same items and the same order.

#### Scenario: A client that sends nothing

- **WHEN** a list is asked for with no parameters
- **THEN** the answer is the one of before

### Requirement: A list can be paged

A list that says in the contract that it can be paged SHALL accept `limit` (1 to 500) and `offset` (0 or more): it answers at most `limit` items starting at `offset`, in the same body shape, with the header `X-Total-Count` holding how many items match in all and a `Link` header with `rel="next"` and `rel="prev"` when there are such pages. `offset` without `limit` SHALL use a `limit` of 100. A `limit` or an `offset` that is not a number in range SHALL be `400` naming the parameter and the range.

#### Scenario: The second page

- **WHEN** a list of 250 items is asked with `limit=100&offset=100`
- **THEN** the answer has items 101 to 200, `X-Total-Count: 250` and a `Link` header with the next and the previous page

#### Scenario: Beyond the end

- **WHEN** `offset` is greater than the number of items
- **THEN** the answer is `200` with an empty list and the total

#### Scenario: A bad limit

- **WHEN** `limit=0` or `limit=abc` is sent
- **THEN** the answer is `400` naming `limit` and its range

### Requirement: A list can be sorted and searched

A list that can be paged SHALL accept `sort`, the name of one of the fields the contract lists for it, with a `-` before it for descending order, and `q`, a text searched without regard to case in the text fields the contract lists for it. Without `sort` the order SHALL be the one the list has today. A field the contract does not list SHALL be `400` naming the fields that can be used. The filters a list already has SHALL keep working and combine with `q`, `sort`, `limit` and `offset`.

#### Scenario: Newest first

- **WHEN** a list is asked with `sort=-created_at`
- **THEN** the items come newest first

#### Scenario: A search

- **WHEN** a list is asked with `q=neon`
- **THEN** only the items whose listed text fields contain `neon`, in any case, come back and `X-Total-Count` counts them

#### Scenario: A field that cannot be sorted by

- **WHEN** `sort=password` is sent
- **THEN** the answer is `400` and names the fields that can be used
