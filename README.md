
# Static Transaction Chopping for Parallel Smart Contract Executions

```bash
#Usage
go run main.go <inputFile>
```

The phase 1 of the program is used to find potential parallelizable lines in a `Golang` source code file.

If a statement is self-incrementing or self-decrementing or assignment statement, then we find whether the left 
values of them are in the previous conditional statements in the same function definition block.

If so, we check the right-values and make sure they are no need to read state when being used.

If not, then we can parallelize those lines.

The output will be the line numbers of the potential parallelizable lines and where the left values of the line is defined in the same function.

```bash
#example output
[104, 99, 94]
[137, 132, 127, 123]
[154, 153]
[172, 171]
[219, 213]
[240]

```

This means line 104 derives from line 99, and line 99 derived from line 94.

The phase 2 of the program is used to find read/write API calls in a `Golang` source code file.

The output will be the key parameters of the function of the read/write API calls.

```bash
GetState:
CreateAccountRandom: args
CreateAccount: args
Query: args
loadAccount: id

PutState:
saveAccount: account

```